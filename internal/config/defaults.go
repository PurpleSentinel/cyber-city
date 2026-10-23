package config

import "time"

// Defaults returns the built-in configuration. Every value must match
// docs/CONFIG.md and config/cybercity.example.yaml (TestExampleMatchesDefaults).
func Defaults() Config {
	return Config{
		Listen:       "127.0.0.1:8080",
		AllowOrigins: []string{},
		Snaplen:      256,
		Promiscuous:  true,
		Window:       Duration(100 * time.Millisecond),
		MaxHosts:     2000,
		LogLevel:     "info",
		ReplaySpeed:  1,
		Server: ServerConfig{
			MaxClients:   4,
			ClientQueue:  32,
			WriteTimeout: Duration(2 * time.Second),
			PingInterval: Duration(15 * time.Second),
			Heartbeat:    Duration(time.Second),
		},
		Flows: FlowsConfig{MaxTracked: 50000},
		Hosts: HostsConfig{
			IdleTimeout: Duration(120 * time.Second),
			MaxServices: 16,
		},
		Enrich: EnrichConfig{
			HandshakeTimeout: Duration(10 * time.Second),
			RTTSamples:       20,
			RetransWindow:    Duration(30 * time.Second),
			ClassInterval:    Duration(10 * time.Second),
			GatewayMinIPs:    5,
		},
		Detect: DetectConfig{
			Scan: ScanConfig{
				Window:             Duration(10 * time.Second),
				MinPorts:           50,
				MinHosts:           20,
				MinIncompleteRatio: 0.8,
				UDPMinPorts:        100,
				Cooldown:           Duration(60 * time.Second),
			},
			Flood: FloodConfig{
				Window:           Duration(5 * time.Second),
				BaselineHalfLife: Duration(60 * time.Second),
				MinHistory:       Duration(30 * time.Second),
				Multiplier:       10,
				MinPPS:           2000,
				MinSources:       50,
				Cooldown:         Duration(60 * time.Second),
			},
		},
	}
}
