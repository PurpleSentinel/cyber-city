// Command cybercity captures network metadata and serves the 3D city view.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/capture"
	"github.com/PurpleSentinel/cyber-city/internal/pipeline"
	"github.com/PurpleSentinel/cyber-city/internal/server"
	"github.com/PurpleSentinel/cyber-city/web"
)

// Prototype behaviour preserved by T0.2. Milestone 1 makes it safe by default:
// loopback bind (T1.1), auth (T1.2), origin checks (T1.3), privilege drop (T1.6).
const listenAddr = ":8080"

func main() {
	live, err := capture.OpenFirstDevice(65536, true)
	if err != nil {
		log.Fatalf("capture: %v", err)
	}
	defer live.Close()
	log.Printf("Sniffing on device: %s", live.Device)

	agg := pipeline.NewAggregator()
	hub := server.NewHub()

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			flows := agg.Flush()
			if len(flows) == 0 {
				continue
			}
			payload, err := json.Marshal(flows)
			if err != nil {
				log.Printf("marshal: %v", err)
				continue
			}
			hub.Broadcast(payload)
		}
	}()

	go func() {
		log.Printf("Serving on %s", listenAddr)
		srv := &http.Server{
			Addr:              listenAddr,
			Handler:           server.Handler(hub, web.Dist()),
			ReadHeaderTimeout: 10 * time.Second,
		}
		log.Fatal(srv.ListenAndServe())
	}()

	for packet := range live.Packets() {
		agg.Add(packet)
	}
}
