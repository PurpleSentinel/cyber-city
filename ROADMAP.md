# CyberCity-3D Roadmap

Goal: keep the core idea — a Go service captures live traffic and streams it to a three.js city where hosts are buildings and activity drives height — while making the project safe by default, correct, and verifiable.

This page is the overview. The build is specified in:

| Doc | Contents |
|---|---|
| [docs/DESIGN.md](docs/DESIGN.md) | The concept and what passive capture can really deliver |
| [docs/TASKS.md](docs/TASKS.md) | PR-sized tasks with dependencies and "Done when" checks: **the source of truth for progress** |
| [docs/PROTOCOL.md](docs/PROTOCOL.md) | Backend ↔ browser contract (endpoints, auth, message schema) |
| [docs/CONFIG.md](docs/CONFIG.md) | Flags, defaults, thresholds, classification rules |
| [docs/VISUAL_SPEC.md](docs/VISUAL_SPEC.md) | Palette, layout geometry, formulas, effect timings, HUD |
| [docs/TESTING.md](docs/TESTING.md) | Fixtures, golden tests, security tests, e2e, CI |
| [docs/DECISIONS.md](docs/DECISIONS.md) | Settled choices |
| [AGENTS.md](AGENTS.md) | How to work in this repo (commands, workflow, hard rules) |

## Principles
- **Safe by default**: with no flags, nothing is reachable except from this machine, by this visualizer.
- **Honest claims**: the README describes what it actually does. Heuristics are labelled as heuristics; this is a visualizer, not an IDS.
- **Metadata only**: packet payloads are never inspected or transmitted.
- **Verifiable**: tests, CI, synthetic pcap fixtures and a replay demo anyone can reproduce.

## Milestone 0: Foundations (T0.x)
Toolchain and Makefile, restructure into one Go module plus `web/` (TypeScript), CI, protocol types with cross-language conformance tests, the config package, the OUI vendor table, a synthetic pcap fixture generator, and `--read` replay with packet-time windows and golden tests. Everything after this can be verified by CI.

## Milestone 1: Safe by default (T1.x)
| Problem | Fix |
|---|---|
| `CheckOrigin` returns true (any website can read the feed) | Strict Origin allow-list; the frontend is served from the Go binary via `embed`, so page and WebSocket share one origin. |
| Listens on `:8080` (all interfaces) | Default bind `127.0.0.1:8080`; `--listen` with a loud warning for non-loopback. |
| No authentication | A per-run token printed as `http://127.0.0.1:8080/auth?token=…`, exchanged for an HttpOnly, SameSite=Strict session cookie; `/ws` requires the cookie and an allowed Origin. |
| `sudo go run` | Build a binary and grant `setcap cap_net_raw,cap_net_admin+eip`; if started as root, open the capture handle, then drop to `nobody`. |
| Privacy | Payloads never sent; snaplen 256; MAC addresses opt-in; `--anonymize` for screen recordings. |
| Slow-client stall | Per-client queues with write deadlines; slow clients dropped; read limit; max clients. |
| Supply chain | Maintained `gopacket/gopacket` fork, `go.sum`, Dependabot, `govulncheck`, SECURITY.md. |

## Milestone 2: Correct, enriched data (T2.x)
Interface selection, BPF filters, IPv6/ICMP/VLAN decoding, flow keying with service-port rules and traffic categories, the host table, then passive enrichment: active services, handshake RTT, retransmissions, MAC vendor with gateway detection, heuristic device class, districts (config, VLAN or automatic), optional offline GeoIP sectors, and capture drop stats.

## Milestone 3: The city (T3.x)
- **Districts**: Corporate/LAN grid, Server Core at the centre, the Outlands ring placed by compass sector.
- **Buildings**: height follows the current bandwidth (log scale, decays when quiet); facade windows show active services; colour shows device class; shaders show degraded, spike and lightning states.
- **Traffic**: highways between talking hosts; particle freight coloured by category, moving at RTT-based speed; bloom.
- **Interface**: inspector, HUD, software-rendering warning, low-quality mode, a Playwright e2e suite with screenshots.

## Milestone 4: Events (T4.x)
Heuristic detectors and their effects: port scan → Searchlight, flood/DDoS → Meteor Shower, host gone dark → Power Outage. Plus an event log labelled "Heuristic — not an IDS".

## Milestone 5: Credibility and release (T5.x)
Licence (owner's choice), goreleaser builds for Linux amd64/arm64, a README rewrite (recording, architecture diagram, no-root replay quick start, SPAN/TAP visibility, security model, GPU note), and performance baselines.

## Later
- Alternative views on the same data stream: Star Cluster / Node Graph, Radial Sonar / Radar, Particle Vortex (see DESIGN.md).
- WebGPU renderer; Suricata EVE alerts as events; macOS.

## Order
M0 first, because every later task depends on its CI, fixtures and replay. Then M1 on its own, since it removes the real risk. Then M2 and M3 (frontend tasks can run in parallel with backend enrichment once T3.1 is in), then M4 (needs M2's enrichment), then M5.
