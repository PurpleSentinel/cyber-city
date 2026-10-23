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
	if err := notYet(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "cybercity:", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "cybercity:", err)
		os.Exit(1)
	}
}

// notYet rejects settings whose feature has not landed, naming the task that
// adds it (docs/TASKS.md), rather than silently ignoring them.
func notYet(cfg config.Config) error {
	pending := []struct {
		set  bool
		flag string
		task string
	}{
		{cfg.ListIfaces, "--list-ifaces", "T2.1"},
		{cfg.Iface != "", "--iface", "T2.1"},
		{cfg.BPF != "", "--bpf", "T2.2"},
		{cfg.User != "", "--user", "T1.6"},
		{cfg.Districts != "", "--districts", "T2.11"},
		{cfg.GeoIP != "", "--geoip", "T2.12"},
		{cfg.Anonymize, "--anonymize", "T1.7"},
	}
	for _, p := range pending {
		if p.set {
			return fmt.Errorf("%s is not implemented yet (arrives in %s)", p.flag, p.task)
		}
	}
	return nil
}

func run(ctx context.Context, cfg config.Config) error {
	src, info, err := openSource(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	p := pipeline.New(pipeline.Options{
		Window:     cfg.Window.D(),
		Heartbeat:  cfg.Server.Heartbeat.D(),
		MaxTracked: cfg.Flows.MaxTracked,
		Server:     info,
	})

	if cfg.DumpJSON != "" {
		return dumpJSON(ctx, cfg.DumpJSON, src, p)
	}

	ln, err := server.Listen(cfg.Listen, slog.Default())
	if err != nil {
		return err
	}
	url := server.BaseURL(ln.Addr())
	if cfg.URLFile != "" {
		if err := server.WriteURLFile(cfg.URLFile, url); err != nil {
			_ = ln.Close()
			return fmt.Errorf("url file: %w", err)
		}
	}

	hub := server.NewHub(p.Snapshot)
	srv := &http.Server{
		Handler:           server.Handler(hub, web.Dist()),
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
	live, err := capture.OpenFirstDevice(int32(cfg.Snaplen), cfg.Promiscuous) //nolint:gosec // snaplen validated to 128..65535
	if err != nil {
		return nil, info, fmt.Errorf("capture: %w", err)
	}
	slog.Info("capturing", "device", live.Device)
	info.Source, info.Iface = "live", protocol.Ptr(live.Device)
	return live, info, nil
}

func dumpJSON(ctx context.Context, path string, src capture.Source, p *pipeline.Pipeline) (err error) {
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
	sink, err := pipeline.NewJSONLSink(w, p.Snapshot())
	if err != nil {
		return err
	}
	if err := pipeline.Run(ctx, src, p, sink, 0); err != nil {
		return err
	}
	return sink.Close()
}
