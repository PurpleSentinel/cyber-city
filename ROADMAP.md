# CyberCity-3D Roadmap

Goal: keep the core idea — a Go service captures live traffic and streams it to a three.js city where hosts are buildings and activity drives height — while making the project safe by default, correct, and verifiable. The full concept (districts, buildings, highways, events, alternative views) and the feasibility of each mapping are in [docs/DESIGN.md](docs/DESIGN.md).

## Principles
- **Safe by default** — with no flags, nothing is reachable except from this machine, by this visualizer.
- **Honest claims** — the README describes what it actually does, with a screenshot/GIF. Heuristics are labelled as heuristics; this is a visualizer, not an IDS.
- **Metadata only** — packet payloads are never inspected or transmitted.
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

## Milestone 2 — Correct, enriched data
Capture:
- `--iface` / `--list-ifaces`; default to the default-route interface instead of `devices[0]`.
- `--bpf "tcp or udp"` capture filter.
- IPv6 support; 802.1Q VLAN tags parsed.
- Flow key `(proto, src, dst, service port)` where service port is the lower of the two ports; OR TCP flags across packets.
- `--read capture.pcap` replay mode for reproducible demos and tests; the mock generator becomes `--mock` or replays a bundled sample pcap so it cannot drift from the real schema.

Per-host and per-flow enrichment (all passive):
- **Active services** per host: ports seen in use, and services confirmed by the host answering SYN with SYN-ACK.
- **Handshake RTT** from SYN → SYN-ACK → ACK timing; **retransmission / duplicate-sequence counts** as a loss proxy.
- **MAC vendor** via a bundled OUI table; heuristic **device class** (IoT / workstation / server) from vendor and services.
- **Districts** from a `districts.yaml` CIDR → name map (e.g. `10.0.1.0/24: Corporate`, `10.0.9.0/24: Server Core`), VLAN tags, or automatic grouping.
- Optional **GeoIP** sector for external hosts from a user-supplied offline GeoLite2 database (`--geoip path.mmdb`); no external lookups.

Protocol:
- Versioned message schema `{ "v": 1, "ts": …, "flows": [...], "hosts": [...], "events": [...] }`, documented in `docs/protocol.md`.

## Milestone 3 — The city
Layout:
- **Districts**: Corporate/LAN as a dense orderly grid, Server Core as monoliths behind force-fields, the Outlands as a chaotic ring on the horizon, placed by GeoIP compass sector when available.
- **Stable positions**: each address hashes to a fixed slot within its district, so buildings never overlap and the city is recognisable across sessions.

Buildings:
- **Height = current bandwidth**: bytes/sec with exponential decay on a log scale, so the skyline breathes rather than saturating.
- **Facade windows = active services**: e.g. a blue column for 80/443, a blinking white segment for 22.
- **Colour = device class** (heuristic, labelled as such in the inspector).
- **Shaders**: holographic cyan when normal; digital-static/VHS artefacts when RTT or retransmissions are high; a ground-to-roof white flash on traffic spikes; `UnrealBloomPass` for the neon look.

Traffic:
- **Highways** between hosts that are talking, with lane width = throughput.
- **Particle freight** for aggregated packets: green HTTPS, yellow DNS, red cleartext/legacy/unknown. Speed follows the handshake RTT when known.

Interface:
- **Hover/click inspector**: IP, MAC vendor, district, services, bytes/sec, RTT; optional reverse DNS, off by default.
- **Lifecycle**: idle hosts fade out; least-recently-used eviction at capacity; capped particle counts.
- **HUD**: packets/sec, host count, connection state; auto-reconnect with backoff; pause.

## Milestone 4 — Events
Backend detectors emit `events`; the frontend plays a cinematic effect and lists each event, with its evidence, in an on-screen log. Thresholds are configurable.
- **Port scan → the Searchlight**: one source probing many ports/hosts with mostly incomplete handshakes; a red searchlight sweeps from the scanner across its targets.
- **Flood / DDoS → the Meteor Shower**: rate to one destination far above its baseline from many sources; red sky, particles rain on the target, force-field flickers.
- **Host gone dark → the Power Outage**: idle past timeout; windows flicker, the building dissolves into the grid.
- Tests replay crafted pcaps (e.g. an `nmap -sS` capture) and assert the expected events.
- The UI and README state that these are heuristics and point to Suricata/Zeek for real detection; the event schema allows external alerts to be fed in later.

## Milestone 5 — Credibility and release
- Layout: `cmd/cybercity`, `internal/capture`, `internal/aggregate`, `internal/enrich`, `internal/detect`, `internal/server`, `web/` (embedded).
- Tests: Go unit tests with crafted packets, pcap-fixture golden tests, origin/token rejection tests; Vitest for layout and decay logic.
- CI: `go vet`, `golangci-lint`, `govulncheck`, tests, frontend build.
- Releases via goreleaser; `LICENSE`; `SECURITY.md`.
- README: GIF, architecture diagram, no-root quick start (`--read sample.pcap`), security model, and **deployment visibility**: on a switched network promiscuous mode sees only broadcast/multicast and the host's own traffic, so whole-network views need a SPAN/mirror port or a network TAP.

## Later
- Alternative views on the same data stream: Star Cluster / Node Graph, Radial Sonar / Radar, Particle Vortex (see DESIGN.md).
- WebGPU renderer.

## Order
Ship Milestone 1 as its own change first, since it removes the real risk. Then Milestones 2 and 3 together (replay mode makes the visuals testable), then Milestone 4 (events depend on the enrichment from Milestone 2), then Milestone 5.
