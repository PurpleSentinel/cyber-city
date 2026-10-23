// Command cybercity captures network metadata and serves the 3D city view.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/buildinfo"
	"github.com/PurpleSentinel/cyber-city/internal/capture"
	"github.com/PurpleSentinel/cyber-city/internal/config"
	"github.com/PurpleSentinel/cyber-city/internal/pipeline"
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
	run(cfg)
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
		{cfg.Read != "", "--read", "T0.8"},
		{cfg.DumpJSON != "", "--dump-json", "T0.8"},
		{cfg.BPF != "", "--bpf", "T2.2"},
		{cfg.URLFile != "", "--url-file", "T1.1"},
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

// run is still the prototype loop; T0.8 replaces it with internal/pipeline.Run.
func run(cfg config.Config) {
	live, err := capture.OpenFirstDevice(int32(cfg.Snaplen), cfg.Promiscuous) //nolint:gosec // snaplen validated to 128..65535
	if err != nil {
		log.Fatalf("capture: %v", err)
	}
	defer live.Close()
	slog.Info("capturing", "device", live.Device)

	agg := pipeline.NewAggregator()
	hub := server.NewHub()

	go func() {
		ticker := time.NewTicker(cfg.Window.D())
		defer ticker.Stop()
		for range ticker.C {
			flows := agg.Flush()
			if len(flows) == 0 {
				continue
			}
			payload, err := json.Marshal(flows)
			if err != nil {
				slog.Error("marshal", "err", err)
				continue
			}
			hub.Broadcast(payload)
		}
	}()

	go func() {
		slog.Info("serving", "url", "http://"+cfg.Listen)
		srv := &http.Server{
			Addr:              cfg.Listen,
			Handler:           server.Handler(hub, web.Dist()),
			ReadHeaderTimeout: 10 * time.Second,
		}
		log.Fatal(srv.ListenAndServe())
	}()

	for packet := range live.Packets() {
		agg.Add(packet)
	}
}
