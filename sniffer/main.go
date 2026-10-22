package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/gorilla/websocket"
)

type PacketMetrics struct {
	Timestamp   int64    `json:"timestamp"`
	SrcIP       string   `json:"src_ip"`
	SrcMac      string   `json:"src_mac"`
	SrcIsLocal  bool     `json:"src_is_local"`
	DstIP       string   `json:"dst_ip"`
	DstMac      string   `json:"dst_mac"`
	DstIsLocal  bool     `json:"dst_is_local"`
	Protocol    string   `json:"protocol"`
	Port        int      `json:"port"`
	PacketCount int      `json:"packet_count"`
	ByteCount   int      `json:"byte_count"`
	Flags       []string `json:"flags"`
}

type Aggregator struct {
	mu      sync.Mutex
	metrics map[string]*PacketMetrics
}

var (
	aggregator = &Aggregator{metrics: make(map[string]*PacketMetrics)}
	upgrader   = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	clients    = make(map[*websocket.Conn]bool)
	clientsMu  sync.Mutex
)

func main() {
	// Find default device
	devices, err := pcap.FindAllDevs()
	if err != nil || len(devices) == 0 {
		log.Fatalf("No devices found: %v", err)
	}
	device := devices[0].Name
	fmt.Printf("Sniffing on device: %s\n", device)

	handle, err := pcap.OpenLive(device, 65536, true, 100*time.Millisecond)
	if err != nil {
		log.Fatalf("Error opening live interface: %v", err)
	}
	defer handle.Close()

	// Broadcast loop (every 100ms)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		for range ticker.C {
			aggregator.mu.Lock()
			if len(aggregator.metrics) == 0 {
				aggregator.mu.Unlock()
				continue
			}
			payload, _ := json.Marshal(mapsToSlice(aggregator.metrics))
			aggregator.metrics = make(map[string]*PacketMetrics) // Reset
			aggregator.mu.Unlock()

			clientsMu.Lock()
			for client := range clients {
				_ = client.WriteMessage(websocket.TextMessage, payload)
			}
			clientsMu.Unlock()
		}
	}()

	// WS Server
	http.HandleFunc("/ws", handleConnections)
	go func() {
		log.Println("WebSocket server starting on :8080...")
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	// Packet Loop
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	for packet := range packetSource.Packets() {
		processPacket(packet)
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	clientsMu.Lock()
	clients[ws] = true
	clientsMu.Unlock()
	defer func() {
		clientsMu.Lock()
		delete(clients, ws)
		clientsMu.Unlock()
		ws.Close()
	}()
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			break
		}
	}
}

func processPacket(packet gopacket.Packet) {
	var srcIP, dstIP, srcMac, dstMac, protocol string
	var port, bytes int
	var flags []string

	netLayer := packet.NetworkLayer()
	if netLayer == nil {
		return
	}
	linkLayer := packet.LinkLayer()
	if linkLayer != nil {
		srcMac = linkLayer.LinkFlow().Src().String()
		dstMac = linkLayer.LinkFlow().Dst().String()
	}

	bytes = len(packet.Data())

	if ip4, ok := netLayer.(*layers.IPv4); ok {
		srcIP = ip4.SrcIP.String()
		dstIP = ip4.DstIP.String()
		protocol = ip4.Protocol.String()
	} else {
		return
	}

	transLayer := packet.TransportLayer()
	if transLayer != nil {
		if tcp, ok := transLayer.(*layers.TCP); ok {
			port = int(tcp.DstPort)
			protocol = "TCP"
			if tcp.SYN { flags = append(flags, "SYN") }
			if tcp.ACK { flags = append(flags, "ACK") }
		} else if udp, ok := transLayer.(*layers.UDP); ok {
			port = int(udp.DstPort)
			protocol = "UDP"
		}
	}

	key := fmt.Sprintf("%s->%s:%d", srcIP, dstIP, port)

	aggregator.mu.Lock()
	defer aggregator.mu.Unlock()

	if m, exists := aggregator.metrics[key]; exists {
		m.PacketCount++
		m.ByteCount += bytes
	} else {
		aggregator.metrics[key] = &PacketMetrics{
			Timestamp:   time.Now().UnixMilli(),
			SrcIP:       srcIP,
			SrcMac:      srcMac,
			SrcIsLocal:  isLocal(srcIP),
			DstIP:       dstIP,
			DstMac:      dstMac,
			DstIsLocal:  isLocal(dstIP),
			Protocol:    protocol,
			Port:        port,
			PacketCount: 1,
			ByteCount:   bytes,
			Flags:       flags,
		}
	}
}

func isLocal(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && (parsed.IsPrivate() || parsed.IsLoopback())
}

func mapsToSlice(m map[string]*PacketMetrics) []*PacketMetrics {
	s := make([]*PacketMetrics, 0, len(m))
	for _, v := range m {
		s = append(s, v)
	}
	return s
}
