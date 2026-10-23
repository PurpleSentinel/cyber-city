// SPDX-License-Identifier: MIT

// Command cybercity captures network metadata and serves the 3D city view.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/buildinfo"
	"github.com/PurpleSentinel/cyber-city/internal/capture"
	"github.com/PurpleSentinel/cyber-city/internal/config"
	"github.com/PurpleSentinel/cyber-city/internal/detect"
	"github.com/PurpleSentinel/cyber-city/internal/districts"
	"github.com/PurpleSentinel/cyber-city/internal/enrich"
	"github.com/PurpleSentinel/cyber-city/internal/pipeline"
	"github.com/PurpleSentinel/cyber-city/internal/protocol"
	"github.com/PurpleSentinel/cyber-city/internal/server"
	"github.com/PurpleSentinel/cyber-city/web"
)

func main() {
	cfg, err := config.Load(os.Args[1:], os.Stderr)
	switch {
	case errors.Is(err, config.ErrHelp):
		return
	case err != nil:
		fmt.Fprintln(os.Stderr, "cybercity:", err)
		os.Exit(2)
	}
	if cfg.ShowVersion {
		fmt.Println("cybercity", buildinfo.Version)
		return
	}
	if cfg.ListIfaces {
		ifs, err := capture.Interfaces()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cybercity:", err)
			os.Exit(1)
		}
		capture.PrintInterfaces(os.Stdout, ifs)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "cybercity:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config) error {
	src, info, err := openSource(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	if cfg.BPF != "" {
		if err := src.SetFilter(cfg.BPF); err != nil {
			return err
		}
	}

	var geo *enrich.Geo
	if cfg.GeoIP != "" {
		g, err := enrich.OpenGeo(cfg.GeoIP, *cfg.Geo.Home)
		if err != nil {
			return err
		}
		defer func() { _ = g.Close() }()
		geo = g
	}
	var matcher *districts.Matcher
	if cfg.Districts != "" {
		defs, err := config.LoadDistricts(cfg.Districts)
		if err != nil {
			return err
		}
		matcher = districts.New(defs)
	}
	p := pipeline.New(pipeline.Options{
		Districts:        matcher,
		Geo:              geo,
		Window:           cfg.Window.D(),
		Heartbeat:        cfg.Server.Heartbeat.D(),
		MaxTracked:       cfg.Flows.MaxTracked,
		MaxHosts:         cfg.MaxHosts,
		IdleTimeout:      cfg.Hosts.IdleTimeout.D(),
		MaxServices:      cfg.Hosts.MaxServices,
		HandshakeTimeout: cfg.Enrich.HandshakeTimeout.D(),
		RTTSamples:       cfg.Enrich.RTTSamples,
		RetransWindow:    cfg.Enrich.RetransWindow.D(),
		GatewayMinIPs:    cfg.Enrich.GatewayMinIPs,
		ClassInterval:    cfg.Enrich.ClassInterval.D(),
		Detectors: []detect.Detector{detect.NewScan(detect.ScanConfig{
			Window:             cfg.Detect.Scan.Window.D(),
			MinPorts:           cfg.Detect.Scan.MinPorts,
			MinHosts:           cfg.Detect.Scan.MinHosts,
			MinIncompleteRatio: cfg.Detect.Scan.MinIncompleteRatio,
			UDPMinPorts:        cfg.Detect.Scan.UDPMinPorts,
		}), detect.NewFlood(detect.FloodConfig{
			Window:           cfg.Detect.Flood.Window.D(),
			BaselineHalfLife: cfg.Detect.Flood.BaselineHalfLife.D(),
			MinHistory:       cfg.Detect.Flood.MinHistory.D(),
			Multiplier:       cfg.Detect.Flood.Multiplier,
			MinPPS:           cfg.Detect.Flood.MinPPS,
			MinSources:       cfg.Detect.Flood.MinSources,
		})},
		Cooldowns: map[string]time.Duration{
			protocol.EventPortScan: cfg.Detect.Scan.Cooldown.D(),
			protocol.EventFlood:    cfg.Detect.Flood.Cooldown.D(),
		},
		Server:    info,
		Anonymize: cfg.Anonymize,
	})

	if cfg.DumpJSON != "" {
		return dumpJSON(ctx, cfg, src, p)
	}

	ln, err := server.Listen(cfg.Listen, slog.Default())
	if err != nil {
		return err
	}
	auth, err := server.NewAuth()
	if err != nil {
		_ = ln.Close()
		return err
	}
	url := auth.AuthURL(server.BaseURL(ln.Addr()))
	if cfg.URLFile != "" {
		if err := server.WriteURLFile(cfg.URLFile, url); err != nil {
			_ = ln.Close()
			return fmt.Errorf("url file: %w", err)
		}
		giveToSudoUser(cfg.URLFile)
	}
	// Capture handle, listener and URL file are open: give up root (D22).
	if err := dropPrivileges(cfg); err != nil {
		_ = ln.Close()
		return err
	}

	hub := server.NewHub(p.Snapshot, server.AllowedOrigins(ln.Addr(), cfg.AllowOrigins), server.HubOptions{
		MaxClients:   cfg.Server.MaxClients,
		ClientQueue:  cfg.Server.ClientQueue,
		WriteTimeout: cfg.Server.WriteTimeout.D(),
		PingInterval: cfg.Server.PingInterval.D(),
	})
	for _, o := range cfg.AllowOrigins {
		slog.Info("extra origin allowed", "dev_url", auth.AuthURL(o+"/"))
	}
	srv := &http.Server{
		Handler:           server.Handler(hub, web.Dist(), auth),
		ReadHeaderTimeout: 10 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	slog.Info("ready", "url", url)
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	pipeErr := make(chan error, 1)
	go func() { pipeErr <- pipeline.Run(ctx, src, p, hub, cfg.ReplaySpeed) }()
	select {
	case err := <-serveErr:
		return err
	case err := <-pipeErr:
		if err == nil {
			// The capture file ended: keep serving the final state until interrupted.
			slog.Info("replay finished; serving final state (Ctrl-C to exit)")
			select {
			case <-ctx.Done():
				return nil
			case err := <-serveErr:
				return err
			}
		}
		return err
	}
}

func openSource(cfg config.Config) (capture.Source, protocol.ServerInfo, error) {
	info := protocol.ServerInfo{Version: buildinfo.Version, ShowMAC: cfg.ShowMAC}
	if cfg.Read != "" {
		src, err := capture.OpenFile(cfg.Read, cfg.Loop)
		info.Source = "file"
		return src, info, err
	}
	device, err := capture.ResolveIface(cfg.Iface)
	if err != nil {
		return nil, info, err
	}
	live, err := capture.OpenLive(device, int32(cfg.Snaplen), cfg.Promiscuous) //nolint:gosec // snaplen validated to 128..65535
	if err != nil {
		return nil, info, fmt.Errorf("capture: %w", err)
	}
	slog.Info("capturing", "device", live.Device)
	info.Source, info.Iface = "live", protocol.Ptr(live.Device)
	return live, info, nil
}

func dumpJSON(ctx context.Context, cfg config.Config, src capture.Source, p *pipeline.Pipeline) (err error) {
	path := cfg.DumpJSON
	var w io.Writer = os.Stdout
	if path != "-" {
		f, err := os.Create(path) //nolint:gosec // operator-supplied output path
		if err != nil {
			return err
		}
		defer func() {
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}()
		w = f
	}
	if err := dropPrivileges(cfg); err != nil {
		return err
	}
	sink, err := pipeline.NewJSONLSink(w, p.Snapshot())
	if err != nil {
		return err
	}
	if err := pipeline.Run(ctx, src, p, sink, 0); err != nil {
		return err
	}
	return sink.Close()
}
