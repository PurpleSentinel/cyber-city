# CyberCity-3D Roadmap

Goal: keep the core idea — a Go service captures live traffic and streams it to a three.js city where hosts are buildings and activity drives height — while making the project safe by default, correct, and verifiable.

## Principles
- **Safe by default** — with no flags, nothing is reachable except from this machine, by this visualizer.
- **Honest claims** — the README describes what it actually does, with a screenshot/GIF.
- **Verifiable** — tests, CI, and a pcap-replay demo anyone can reproduce.

## Milestone 1 — Safe by default
| Problem | Fix |
|---|---|
| `CheckOrigin` returns true (any website can read the feed) | Strict origin allow-list; serve the frontend from the Go binary via `embed` so page and WebSocket share one origin. |
| Listens on `:8080` (all interfaces) | Default bind `127.0.0.1:8080`; `--listen` flag with a loud warning for non-loopback. |
| No authentication | Random per-run token printed as a ready-to-open URL (`http://127.0.0.1:8080/?token=…`); WebSocket rejects requests without it. |
| `sudo go run` | Build a binary and grant `setcap cap_net_raw,cap_net_admin+eip`; if started as root, open the capture handle then drop privileges. |
| Privacy | Payloads never sent (already true); MAC addresses opt-in; `--anonymize` hashes/truncates IPs for screen recordings. |
| Slow-client stall | Per-client buffered send channel with write deadlines, drop slow clients; `SetReadLimit`; max client count. |
| Unmaintained dependency | Move from `google/gopacket` to the maintained `gopacket/gopacket` fork; commit `go.sum`; enable Dependabot. |

## Milestone 2 — Correct data
- `--iface` / `--list-ifaces`; default to the default-route interface instead of `devices[0]`.
- `--bpf "tcp or udp"` capture filter.
- IPv6 support.
- Flow key `(proto, src, dst, service port)` where service port is the lower of the two ports; OR TCP flags across packets.
- `--read capture.pcap` replay mode for reproducible demos and tests; the mock generator becomes `--mock` or replays a bundled sample pcap so it cannot drift from the real schema.
- Versioned message schema `{ "v": 1, "ts": …, "flows": [...] }`, documented in `docs/protocol.md`.

## Milestone 3 — The city
- **Height = current rate**, not lifetime total: bytes/sec with exponential decay on a log scale, so buildings rise and fall.
- **Stable layout**: local hosts in a downtown grid; external hosts in districts by /16; the IP hashes to a fixed slot, so buildings never overlap.
- **Colour by protocol** (HTTPS / DNS / SSH / other), emissive windows, `UnrealBloomPass`.
- **Flows as light trails** along roads between the two hosts, using the protocol/port data currently ignored.
- **Hover/click inspector**: IP, ports, bytes/sec, protocol; optional reverse DNS, off by default.
- **Lifecycle**: idle hosts fade out; least-recently-used eviction at capacity.
- **HUD**: packets/sec, host count, connection state; auto-reconnect with backoff; pause.

## Milestone 4 — Credibility and release
- Layout: `cmd/cybercity`, `internal/capture`, `internal/aggregate`, `internal/server`, `web/` (embedded).
- Tests: Go unit tests with crafted packets, pcap-fixture golden tests, origin/token rejection tests; Vitest for layout and decay logic.
- CI: `go vet`, `golangci-lint`, `govulncheck`, tests, frontend build.
- Releases via goreleaser; `LICENSE`; `SECURITY.md`.
- README: GIF, architecture diagram, no-root quick start (`--read sample.pcap`), what promiscuous mode can see on switched and Wi-Fi networks, security model.

## Order
Ship Milestone 1 as its own change first, since it removes the real risk. Then Milestones 2 and 3 together (replay mode makes the visuals testable), then Milestone 4.
