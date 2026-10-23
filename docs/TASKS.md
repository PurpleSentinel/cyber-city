# Tasks

The build, broken into PR-sized tasks. Work top to bottom within a milestone unless the **Depends** lines allow parallel work. Read AGENTS.md first.

How to use this file:
- One task per branch and PR: branch `task/T1.2-session-cookie`, PR title `T1.2: Session cookie auth`.
- In the same PR, tick the task's checkbox here and add any new decisions to DECISIONS.md.
- **Done when** is the acceptance test. CI must be green (TESTING.md). If a check can't be met as written, stop and record why in the PR rather than weakening the check.
- 👤 means a human decision or action; agents prepare the PR or a question, but never decide these.

Spec abbreviations: **P** = PROTOCOL.md, **C** = CONFIG.md, **V** = VISUAL_SPEC.md, **T** = TESTING.md, **D** = DECISIONS.md.

---

## Milestone 0: Foundations

### [x] T0.1 Toolchain and repo hygiene
- **Depends:** none. **Touches:** `Makefile`, `.gitignore`, `.editorconfig`, `.golangci.yml`, `.nvmrc`, `go.mod`.
- **Do:**
  - Root `go.mod` (`module github.com/PurpleSentinel/cyber-city`, Go ≥ 1.25 with a `toolchain` line).
  - `.nvmrc` with `22`.
  - Makefile targets exactly as listed in AGENTS.md (stubs may echo "not yet" until their task lands).
  - `.gitignore` covering `bin/`, `web/node_modules/`, `web/dist/*` except `.gitkeep`, `web/e2e/artifacts/`, `*.pcap` outside `testdata/`.
  - `.golangci.yml` enabling govet, staticcheck, errcheck, gosec, revive, gofmt.
- **Done when:** `make help` lists all targets, and `go mod tidy` and `go vet ./...` succeed.

### [x] T0.2 Restructure to the target layout
- **Depends:** T0.1. **Touches:** everything under `sniffer/`, `frontend/`, `tools/generator/`; new `cmd/`, `internal/`, `web/`.
- **Do:**
  - Move the prototype into the AGENTS.md layout with **no behaviour change**:
    - capture code → `internal/capture/live.go`
    - aggregation → `internal/pipeline`
    - WebSocket server → `internal/server`
    - `main` → `cmd/cybercity`
  - Switch to `github.com/gopacket/gopacket` (D1).
  - Frontend: move to `web/`, convert to TypeScript (strict), split into `web/src/main.ts` and `web/src/protocol.ts` (prototype message shape for now).
  - Add `web/embed.go` with `//go:embed all:dist` and commit `web/dist/.gitkeep` (D3).
  - Add ESLint and Vitest configuration.
  - Pin exact versions of current stable three.js, Vite, TypeScript, Vitest and Playwright.
  - Delete `sniffer/`, `frontend/` and `tools/generator/` (D12).
- **Done when:**
  - `make build` produces `bin/cybercity` serving the built frontend on `/`.
  - `go vet ./...`, `tsc --noEmit` and `vitest run` (one placeholder test) pass.
  - README "Getting started" is updated to `make build` and `sudo bin/cybercity`. The old security warning stays until M1.

### [x] T0.3 Continuous integration
- **Depends:** T0.2. **Touches:** `.github/workflows/ci.yml`, `.github/dependabot.yml`.
- **Do:** create the `go` and `web` jobs from T: CI. Leave `e2e` and `live-smoke` as jobs that are skipped until T3.12 and T1.8. Configure Dependabot for gomod, npm (`/web`) and github-actions, weekly.
- **Done when:** CI runs green on the PR. 👤 Owner enables branch protection requiring CI on `main`.

### [x] T0.4 Protocol types and conformance
- **Depends:** T0.2. **Touches:** `internal/protocol`, `web/src/protocol.ts`, `web/test/protocol.test.ts`, `testdata/protocol/`.
- **Do:**
  - Go structs for every P message and object, with JSON tags, `null` handling (pointer or nullable types) and empty arrays never marshalling as `null`.
  - Sort helpers implementing P: *Ordering*.
  - Example builders and an `-update` flag that writes `testdata/protocol/*.json`.
  - Mirror the types in TS, plus a small runtime validator (no schema library needed).
- **Done when:** the Go round-trip test passes, the TS conformance test passes, and changing a field name on only one side fails CI.

### [x] T0.5 Configuration package
- **Depends:** T0.2. **Touches:** `internal/config`, `config/cybercity.example.yaml`.
- **Do:** implement the defaults, YAML loading (`gopkg.in/yaml.v3`), flags and precedence from C. Validate ranges (window 50ms–1s, snaplen ≥ 128, district id regex, `geoip` requires `geo.home`, `--read` and `--iface` mutually exclusive, `--dump-json` requires `--read`).
- **Done when:**
  - A test loads `config/cybercity.example.yaml` and asserts it equals the defaults.
  - There is one table test per validation error.
  - `cybercity --help` shows every flag in C.

### [x] T0.6 OUI vendor table
- **Depends:** T0.2. **Touches:** `internal/enrich/oui*`, `tools/ouigen`.
- **Do:**
  - `tools/ouigen` downloads (or reads with `-in`) the IEEE MA-L CSV and writes `internal/enrich/oui_table.go`: a sorted `[]struct{prefix uint32; vendor string}` with binary search, vendor names trimmed.
  - Add `//go:generate go run ../../tools/ouigen` and commit the generated file.
  - `Lookup(mac) (vendor string, ok bool)` ignores locally administered MACs (bit 0x02 of the first octet).
- **Done when:**
  - Tests pass: `b8:27:eb:..` → contains "Raspberry Pi", `00:00:0c:..` → contains "Cisco", `02:00:00:..` → not found.
  - The generated file is < 3 MB.

### [x] T0.7 Fixture generator
- **Depends:** T0.6. **Touches:** `tools/fixturegen`, `testdata/pcap`, `testdata/geo`.
- **Do:** generate every fixture in T: *Fixtures* with `gopacket.SerializeLayers` and `pcapgo.NewWriter`, plus `test-city.mmdb` via `mmdbwriter`. Follow the determinism rules. `make fixtures` runs it.
- **Done when:**
  - Running it twice gives byte-identical output (a test checks hashes).
  - Total size is < 10 MB.
  - A test reads each pcap back and checks packet counts and time spans against a table in fixturegen.

### [x] T0.8 File replay and the golden harness
Split into two PRs (AGENTS.md size rule):
- [x] **T0.8a Decode and flow keying**: `DecodingLayerParser` decoder (Ethernet, 802.1Q, IPv4/6, TCP, UDP, ICMP/ICMPv6) producing header-length-based records; category table; service-port rules with a bounded per-connection cache. This pulls forward the decoding part of T2.3 and the keying part of T2.4; those tasks keep their remaining "Done when" checks.
- [x] **T0.8b Replay, windows and golden**: the rest of this task.

- **Depends:** T0.4, T0.5, T0.7. **Touches:** `internal/capture/file.go`, `internal/pipeline`, `cmd/cybercity`.
- **Do:**
  - `Source` interface; `--read`, `--replay-speed`, `--loop`, `--dump-json`.
  - Aggregation windows driven by packet timestamps (D6); heartbeat ticks.
  - Emit P `hello`/`tick` with flows and hosts so far: no enrichment, categories via the C table, service port via C rules.
  - `bytes` from on-wire length (D8).
  - Golden test harness with `-update`.
- **Done when:**
  - Golden files exist for every fixture.
  - Two runs give identical output.
  - Lengths in `web_https` reflect on-wire length even at snaplen 128 (a test checks this).

---

## Milestone 1: Safe by default

### [x] T1.1 Loopback bind and remote warning
- **Depends:** T0.5. **Touches:** `internal/server`, `cmd/cybercity`.
- **Do:** default `127.0.0.1:8080`; `--listen` with port 0 support; WARN for non-loopback hosts (C wording); `--url-file`.
- **Done when:** the T security table rows for `--listen` pass.

### [x] T1.2 Token and session cookie
- **Depends:** T1.1. **Touches:** `internal/server/auth.go`, `web/src/net/session.ts`.
- **Do:**
  - Per-run 32-byte token, printed as `http://<addr>/auth?token=…`.
  - `/auth` (including `next` validation) and `/api/session` per P.
  - The frontend calls `/api/session` first and shows `Invalid link` on `401`.
- **Done when:** the T rows for `/auth` and `/api/session` pass. Manual check (described in the PR): opening the printed URL lands on `/` with no token in the address bar.

### [x] T1.3 Origin check and dev proxy
- **Depends:** T1.2. **Touches:** `internal/server`, `web/vite.config.ts`, `Makefile` (`dev`).
- **Do:**
  - The upgrader's `CheckOrigin` allows only `http://<listen addr>` and `--allow-origin` values.
  - Vite proxies `/auth`, `/api` and `/ws` (with `ws: true`) to the backend.
  - `make dev` runs the backend (`--read demo.pcap --loop --allow-origin http://localhost:5173`) and `vite` together.
- **Done when:** the T `/ws` Origin rows pass and `make dev` serves a working page at the printed dev URL.

### [x] T1.4 Static serving and security headers
- **Depends:** T1.2. **Touches:** `internal/server/static.go`, `web/index.html`, `web/src/ui/styles.css`.
- **Do:** serve the embedded `dist` with the P headers. Move all inline styles and scripts out of `index.html`.
- **Done when:** the T header row passes, and the built page loads with no CSP violations in the browser console (checked in e2e once T3.12 lands; until then, manual and described in the PR).

### [x] T1.5 Client fan-out hardening
- **Depends:** T1.2. **Touches:** `internal/server/hub.go`.
- **Do:**
  - A hub with a per-client buffered channel (`client_queue`), a writer goroutine per client with `write_timeout`, close code 1013 on overflow.
  - `SetReadLimit(512)`, ping/pong, `max_clients` → 503.
  - The broadcaster never holds a lock while writing to a socket.
- **Done when:** the T slow-client, oversize-message and max-clients rows pass under `-race`.

### [x] T1.6 Privilege handling
- **Depends:** T0.8. **Touches:** `internal/privdrop`, `cmd/cybercity`, `Makefile` (`setcap`), README.
- **Do:**
  - Follow the C startup order.
  - When euid is 0: drop to `--user` (default `nobody`) using `setgroups`, `setgid` and `setuid`, and verify the drop with `os.Getuid`. `--user root` logs a WARN.
  - `make setcap` runs `sudo setcap cap_net_raw,cap_net_admin+eip bin/cybercity`.
  - Document both ways to run in the README.
- **Done when:**
  - A unit test covers user lookup and the error paths.
  - The live-smoke CI job (T1.8) asserts that the process uid ≠ 0 after startup when launched with sudo.

### [x] T1.7 Privacy controls
- **Depends:** T0.8. **Touches:** `internal/pipeline`, `internal/capture`, `internal/protocol`.
- **Do:** snaplen 256 default; `--show-mac`; `--anonymize` (HMAC ids, `ip: null`, anonymous districts, test hook for a fixed key).
- **Done when:** golden `web_https --anonymize` contains no `10.0.1.` or `203.0.113.` substrings anywhere, and `--show-mac` toggles `mac` fields in golden output.

### [x] T1.8 Live capture smoke test and SECURITY.md
- **Depends:** T1.6. **Touches:** `internal/capture/live_test.go` (`//go:build live`), CI, `SECURITY.md`.
- **Do:**
  - The test captures on `lo` while it pings 127.0.0.1 and curls a local `httptest` server, and asserts that at least one flow arrives.
  - Enable the live-smoke CI job.
  - `SECURITY.md`: supported versions, how to report (GitHub private vulnerability reporting), the threat model summary from DESIGN.md.
- **Done when:** the live-smoke job is green.

**After M1:** remove the "Security" warning from the README (done in T1.8).

---

## Milestone 2: Correct, enriched data

### [x] T2.1 Interface selection
- **Depends:** T0.8. **Touches:** `internal/capture/ifaces.go`.
- **Do:**
  - `--list-ifaces`.
  - The default interface is the one carrying the default route: parse `/proc/net/route` and `/proc/net/ipv6_route` on Linux; fall back to the first up, non-loopback interface with an address.
  - Fail clearly if `--iface` doesn't exist.
- **Done when:** a unit test parses a sample `/proc/net/route` and picks the right interface, and `--list-ifaces` marks the default.

### [x] T2.2 BPF filter
- **Depends:** T2.1. **Touches:** `internal/capture`.
- **Do:** `--bpf` for live capture (`SetBPFFilter`) and file replay (`pcap.NewBPF`, plus a clear error when libpcap is unavailable). An invalid expression fails at startup.
- **Done when:** a golden-style test of `--read dns.pcap --bpf "tcp"` yields zero flows.

### [x] T2.3 IPv6, ICMP and VLAN decoding
- **Depends:** T0.8. **Touches:** `internal/pipeline/decode.go`.
- **Do:** use `DecodingLayerParser` for Ethernet, Dot1Q, IPv4, IPv6, TCP, UDP, ICMPv4 and ICMPv6, and keep the VLAN id on the packet record. Local-address rule from C.
- **Done when:** the `ipv6` fixture assertions in T pass (the `vlan` district assertion needs the districts file and is checked in T2.11); decode benchmark ≥ 1M packets/s on CI hardware (`go test -bench`).

### [x] T2.4 Flow keying and categories
- **Depends:** T2.3. **Touches:** `internal/pipeline/flow.go`.
- **Do:** flow key, service-port rules, `server_side`, category table, TCP flag union, `flows.max_tracked` eviction (all from C).
- **Done when:** table tests cover every service-port rule and every category row, and golden files are updated.

### [x] T2.5 Host table
- **Depends:** T2.4. **Touches:** `internal/hosts`.
- **Do:**
  - `first_seen`/`last_seen`, the `local` flag, "changed" semantics (P), `removed_hosts`.
  - `--max-hosts` eviction of the least recently seen; `idle_timeout` removal.
  - Snapshot for `hello`.
- **Done when:** unit tests for change detection and eviction pass, and the `idle` fixture has `10.0.1.30` in `removed_hosts` at ≈ 130 s (the event comes in T4.4).

### [x] T2.6 Services
- **Depends:** T2.5. **Touches:** `internal/enrich/services.go`.
- **Do:** add services from flows where this host is the server side; set `confirmed` per P; cap at `max_services`.
- **Done when:** the `web_https`, `dns` and `ssh` service assertions in T pass.

### [x] T2.7 Handshake RTT
- **Depends:** T2.4. **Touches:** `internal/enrich/rtt.go`.
- **Do:** the RTT algorithm in C; flow `rtt_ms`; host median over `rtt_samples`; pending-SYN expiry.
- **Done when:** `web_https` has `rtt_ms` 12.0 ± 0.5, and a unit test shows a retransmitted SYN gives no sample.

### [x] T2.8 Retransmissions
- **Depends:** T2.4. **Touches:** `internal/enrich/retrans.go`.
- **Do:** the retransmission algorithm in C; flow `retrans`; host `retrans_rate` over `retrans_window`.
- **Done when:** `retrans` has `retrans_rate` 0.05 ± 0.01, and `web_https` has 0.

### [x] T2.9 MAC, vendor and gateway detection
- **Depends:** T0.6, T2.5. **Touches:** `internal/enrich/mac.go`.
- **Do:** attribute MACs and vendors per D13, detect gateways, gate `mac` behind `--show-mac`.
- **Done when:** the `gateway` and `web_https` vendor assertions in T pass, and `--show-mac` toggles `mac` fields in golden output (moved here from T1.7: MACs are first attributed in this task).

### [x] T2.10 Device class
- **Depends:** T2.6, T2.9. **Touches:** `internal/enrich/class.go`.
- **Do:** the C rules in order, re-evaluated every `class_interval`, with vendor lists as package-level data.
- **Done when:** a table test has one case per rule plus precedence cases, and `demo` golden output has at least one host of every class.

### [x] T2.11 Districts
- **Depends:** T2.3, T2.5. **Touches:** `internal/districts`.
- **Do:** load the districts file, VLAN-then-CIDR matching, auto districts and anonymous naming (P), and late-district announcement in ticks.
- **Done when:** the `vlan` fixture assertions pass, and auto-district ids match P for `web_https` without a districts file.

### [x] T2.12 GeoIP sectors
- **Depends:** T2.5, T0.7. **Touches:** `internal/enrich/geo.go`.
- **Do:** optional GeoIP lookup and the bearing → sector rule (C); `geo` only for non-local hosts; `server.geoip` in `hello`.
- **Done when:** with the test mmdb and home `[51.5, −0.12]`, JP → NE, US → W, DE → E, and without `--geoip` every `geo` is `null`.

### [x] T2.13 Capture stats
- **Depends:** T2.1. **Touches:** `internal/capture`.
- **Do:** fill `stats.kernel_drops` and `if_drops` from `handle.Stats()` once per second (0 for files).
- **Done when:** a unit test with a fake stats provider passes, and the live-smoke test asserts the fields exist.

---

## Milestone 3: The city

### [x] T3.1 Frontend architecture and connection
- **Depends:** T0.4, T1.2. **Touches:** `web/src/**`.
- **Do:**
  - Create the module layout from AGENTS.md and `web/src/config.ts` with every V constant.
  - Write the connection manager: session check, `hello`/`tick` handling, seq-gap reconnect, backoff (V).
  - Write the store: hosts, districts, rates.
  - Add the debug hook (V).
  - Detect the renderer and show the software warning, falling back to low quality.
- **Done when:** Vitest covers backoff timing (with fake timers), the seq-gap → reconnect path and the hello-rebuilds-state path.

### [x] T3.2 Rate and height model
- **Depends:** T3.1. **Touches:** `web/src/state/rate.ts`.
- **Do:** EWMA, decay between ticks, target height, displayed height (all from V).
- **Done when:** Vitest checks the V numeric examples (1 kB/s ≈ 2.8, 1 MB/s ≈ 19, 100 MB/s ≈ 31, cap 40) and that a height halves toward 1 within the half-life with no ticks.

### [x] T3.3 Deterministic layout
- **Depends:** T3.1. **Touches:** `web/src/layout/*`.
- **Do:** fnv1a, block probing, slot probing, Outlands rings and sectors (V).
- **Done when:** Vitest shows 2000 random ids give no overlapping footprints, the same ids in the same order give identical positions and each host takes its home slot unless already taken (D24; most hosts are then order-independent), server districts take the centre block, and sector placement follows V.

### [x] T3.3a Lighting rig, ground and district plots
- **Depends:** T3.1, T3.3. **Touches:** `web/src/render/scene.ts`, `web/src/render/ground.ts`, `web/eslint.config.js`.
- **Do:**
  - Implement V *Lighting, materials and shading*: renderer tone mapping and colour space, hemisphere light, shadow-casting moon, RoomEnvironment PMREM, lit ground, grid overlay, district plots with emissive borders.
  - Add the ESLint `no-restricted-syntax` guard against wireframe/basic/edges materials (T *Shading guards*).
  - Delete the prototype's `GridHelper`-only floor and wireframe material.
- **Done when:**
  - The Vitest shading guard for scene, ground and plots passes.
  - ESLint fails on a deliberately added `wireframe: true` (shown in the PR, then removed).
  - A screenshot in the PR shows lit plots and ground.

### [x] T3.4 Buildings
- **Depends:** T3.2, T3.3a. **Touches:** `web/src/render/buildings.ts`, `web/src/render/materials.ts`.
- **Do:**
  - The instanced mesh with the **lit dark-glass building material** (V): albedo, roughness and metalness, edge accents, fresnel rim, roof cap.
  - `castShadow`, `receiveShadow` and a matching `customDepthMaterial`.
  - Per-instance attributes, device-class colours, fade-in and fade-out, instance free list.
  - Debug hook fields `lit`, `wireframe`, `shadows`, plus `probe` and `pose`.
- **Done when:**
  - The debug hook's `buildings` count matches the store's host count in a Vitest scene test.
  - The Vitest shading guard for the building material and mesh passes.
  - A PR screenshot at `pose=auto` shows solid, lit buildings casting shadows.
  - The e2e shading test (T step 5) passes once T3.12 lands.

### [x] T3.5 Facade windows
- **Depends:** T3.4. **Touches:** the building shader.
- **Do:** service-category columns and the SSH blink (V).
- **Done when:** a unit test checks the bitmask packing, and a shader compile check passes (renderer `compile()` in e2e).

### [x] T3.6 Status shaders and lightning
- **Depends:** T3.5. **Touches:** the building shader, `web/src/render/effects/lightning.ts`.
- **Do:** degraded noise, spike flash, lightning, with triggers per V.
- **Done when:** Vitest covers the trigger functions (degraded formula, spike and lightning thresholds).

### [x] T3.7 Highways
- **Depends:** T3.4. **Touches:** `web/src/render/highways.ts`.
- **Do:** pair aggregation, routing, width, dominant colour, fade, the 500 cap (V).
- **Done when:** Vitest covers the L-route geometry, width formula, cap selection and fade timing.

### [x] T3.8 Particle freight
- **Depends:** T3.7. **Touches:** `web/src/render/particles.ts`.
- **Do:** the pool, emission rate, RTT-based travel time, direction (V).
- **Done when:** Vitest covers the emission and travel-time formulas and the pool cap (never more than 5000 live).

### [x] T3.9 Post-processing and quality modes
- **Depends:** T3.4. **Touches:** `web/src/render/post.ts`.
- **Do:** bloom (threshold 0.85, so lit bodies don't bloom), OutputPass, low-quality toggle `L` (drops shadows, bloom and half the particles, but materials stay lit), automatic low quality on software rendering, and the `quality=` debug override.
- **Done when:** the debug hook reports `quality` and `shadows` correctly; under SwiftShader the default is `low` with `lit === true`, and `quality=high` turns shadows on.

### [x] T3.10 Inspector and picking
- **Depends:** T3.4. **Touches:** `web/src/ui/inspector.ts`.
- **Do:** raycast picking, hover tooltip, click panel fields, `F`/`Esc` (V).
- **Done when:** an e2e test clicks the projected screen position of a known host id and the panel shows that id.

### [x] T3.11 HUD, keys and help
- **Depends:** T3.1. **Touches:** `web/src/ui/hud.ts`.
- **Do:** HUD fields, pause, the help overlay (V).
- **Done when:** a Vitest DOM test covers the HUD formatting, and e2e checks that `Space` toggles `Paused`.

### [x] T3.12 End-to-end suite
- **Depends:** T3.4, T3.7, T3.8, T3.9, T1.4. **Touches:** `web/e2e`, CI.
- **Do:** the Playwright flow in T, including the **shading test** (filled faces, face contrast, cast shadow); enable the e2e CI job with the `city.png` and `shading.png` artifacts; check the console for zero CSP violations.
- **Done when:** the e2e job is green, and the artifacts show a solid, lit, shadowed city. Temporarily setting the building material to `wireframe: true` makes the shading test fail (demonstrated in the PR, then reverted).

---

## Milestone 4: Events

### [x] T4.1 Detector framework
- **Depends:** T2.5. **Touches:** `internal/detect`.
- **Do:** a `Detector` interface fed per packet record and per window, event ids, cooldowns, max-64 targets, ordering.
- **Done when:** unit tests cover cooldown suppression and id sequencing.

### [ ] T4.2 Port-scan detector
- **Depends:** T4.1, T2.7. **Touches:** `internal/detect/scan.go`.
- **Do:** the scan rules in C (TCP and UDP).
- **Done when:** the `scan_syn` and `scan_sweep` assertions pass and `web_https`, `dns`, `ssh`, `demo` (except its one scan) have zero `port_scan` events.

### [ ] T4.3 Flood detector
- **Depends:** T4.1. **Touches:** `internal/detect/flood.go`.
- **Do:** the flood rules in C, with the baseline frozen during floods.
- **Done when:** the `flood` assertions pass and no other fixture has `flood` events.

### [ ] T4.4 Host-idle event
- **Depends:** T4.1, T2.5. **Touches:** `internal/detect/idle.go`.
- **Do:** a `host_idle` event for local hosts, in the same tick as their removal.
- **Done when:** the `idle` assertion passes.

### [ ] T4.5 Searchlight effect
- **Depends:** T3.4, T4.2. **Touches:** `web/src/render/effects/searchlight.ts`.
- **Do:** as V; targets get lightning.
- **Done when:** Vitest checks target sequencing and duration, and e2e sees the `demo` scan in `lastEvents()`.

### [ ] T4.6 Meteor-shower effect
- **Depends:** T3.4, T4.3. **Touches:** `web/src/render/effects/meteor.ts`.
- **Do:** as V.
- **Done when:** Vitest checks the sky colour timeline at t = 0, 1, 5, 8 s.

### [ ] T4.7 Power-outage effect
- **Depends:** T3.5, T4.4. **Touches:** `web/src/render/effects/outage.ts`.
- **Do:** as V; free the instance afterwards.
- **Done when:** Vitest checks the dissolve timeline and that the instance returns to the free list.

### [ ] T4.8 Event log
- **Depends:** T3.11, T4.1. **Touches:** `web/src/ui/eventlog.ts`.
- **Do:** as V, including the "not an IDS" footer and the concurrency caps.
- **Done when:** a Vitest DOM test checks formatting and the 50-entry limit, and e2e sees a log entry.

---

## Milestone 5: Credibility and release

### [ ] T5.1 👤 Licence
The owner picks a licence (MIT or Apache-2.0 suggested). An agent then adds `LICENSE` and SPDX headers to source files.

### [ ] T5.2 Release pipeline
- **Depends:** T1.8. **Touches:** `.goreleaser.yaml`, `.github/workflows/release.yml`.
- **Do:** on tag `v*`, build Linux amd64 and arm64 on native runners (cgo + libpcap), with checksums and SBOM, and attach to a GitHub Release.
- **Done when:** a dry run (`goreleaser release --snapshot --clean`) passes in CI. 👤 The owner pushes the first tag.

### [ ] T5.3 README rewrite
- **Depends:** T3.12, T4.8. **Touches:** `README.md`, `docs/media/`.
- **Do:**
  - Recording: a GIF or WebM from the e2e run against `demo.pcap`.
  - An architecture diagram (an SVG or a Mermaid block).
  - A quick start with no root needed (`bin/cybercity --read testdata/pcap/demo.pcap --loop`).
  - A live quick start using `make setcap`.
  - Visibility (SPAN/TAP), the security model, privacy flags, "Heuristic — not an IDS", and the GPU requirement.
- **Done when:** every command in the README is exercised by CI or the e2e job, and the README claims nothing the code doesn't do.

### [ ] T5.4 Performance check
- **Depends:** T3.12. **Touches:** `web/e2e/perf.spec.ts`, `internal/pipeline` benchmarks.
- **Do:** the pipeline sustains ≥ 200k packets/s in replay with `-race` off; e2e records frame time at 2000 hosts (a synthetic many-hosts fixture).
- **Done when:** the numbers are reported in the PR, and regressions of more than 20% against the stored baseline fail a non-blocking CI job.

---

## Later (not scheduled)
- Alternative views: Star Cluster, Radar, Vortex (DESIGN.md).
- WebGPU renderer.
- Ingest external alerts (Suricata EVE JSON) as events.
- macOS support.
