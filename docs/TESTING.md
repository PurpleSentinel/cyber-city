# Testing and Verification

Every task in TASKS.md ends with a "Done when" that maps to something in this document. CI is the source of truth; `make check` runs the same steps locally.

## Layers

| Layer | Tooling | Location | Needs |
|---|---|---|---|
| Go unit tests | `go test` | next to code (`*_test.go`) | Go only |
| Golden pipeline tests | `go test ./internal/pipeline -run Golden` | `testdata/golden/*.jsonl` | Go only (pcapgo) |
| Server/security tests | `net/http/httptest` + gorilla client | `internal/server` | Go only |
| Live-capture smoke test | `go test -tags live ./internal/capture` | | libpcap + `CAP_NET_RAW`; CI runs it with `sudo` on loopback |
| Frontend unit tests | Vitest | `web/test/*.test.ts` | Node |
| Protocol conformance | Vitest reads `testdata/protocol/*.json` | `web/test/protocol.test.ts` | Node |
| End-to-end | Playwright (Chromium) | `web/e2e/*.spec.ts` | built binary + Node |

## Fixtures

All pcaps are **synthetic**, produced by `go run ./tools/fixturegen -out testdata` and committed. Real captures are never committed: they contain other people's metadata.

fixturegen rules:
- Fixed seed, base time `2026-01-01T00:00:00Z`, link type Ethernet. Byte-identical output on every run; CI regenerates the fixtures and fails on any diff.
- Addresses:
  - local hosts: `10.0.1.0/24` (corp), `10.0.9.0/24` (core), `fd00:1::/64`
  - gateway: `10.0.1.1` with MAC `02:00:00:00:00:01`
  - external hosts: only documentation ranges `192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24` and `2001:db8::/32`
- Vendor MACs: fixturegen looks prefixes up by vendor name in the generated OUI table (Raspberry Pi, Espressif, Cisco, Apple, Sonos) and fails if one is missing.
- Packets are headers only (payload lengths are set in the IP/TCP headers, with a zero-filled payload up to 64 bytes) to keep files small. Total `testdata/pcap` size must be < 10 MB.
- fixturegen also writes `testdata/geo/test-city.mmdb`, mapping `203.0.113.0/24` → JP (35.68, 139.69), `198.51.100.0/24` → US (40.71, −74.0) and `192.0.2.0/24` → DE (52.52, 13.40).

| File | Content | Key assertions (besides golden) |
|---|---|---|
| `web_https.pcap` | `10.0.1.10` (Raspberry Pi MAC) makes 3 TCP connections to `203.0.113.10:443` via the gateway. SYN→SYN-ACK 6 ms, SYN-ACK→ACK 6 ms, about 200 kB downloaded. | category `https`, `rtt_ms` 12.0 ± 0.5, `203.0.113.10` service 443 confirmed, external host has `mac: null`, `10.0.1.10` vendor contains "Raspberry Pi" |
| `dns.pcap` | `10.0.1.10` sends 20 UDP queries to `10.0.1.1:53`, each answered | category `dns`, `10.0.1.1` service 53/udp confirmed |
| `ssh.pcap` | `10.0.1.20` → `10.0.9.5:22`, 60 s interactive | category `ssh`, `10.0.9.5` service 22 confirmed |
| `retrans.pcap` | as `web_https`, with 5% of data segments retransmitted | `retrans_rate` 0.05 ± 0.01 for `203.0.113.10` |
| `scan_syn.pcap` | `10.0.1.66` → `10.0.9.5`: SYNs to ports 1–1024 over 8 s; RST for closed ports, SYN-ACK on 22 and 80 | exactly 1 `port_scan`, src `10.0.1.66`, dst `10.0.9.5`, `distinct_ports` ≥ 50 (the event fires as the threshold is crossed, not at the end of the scan) |
| `scan_sweep.pcap` | `10.0.1.66` → `10.0.1.2`–`10.0.1.254`, port 445, no replies | exactly 1 `port_scan`, `dst: null`, `distinct_hosts` ≥ 20, `targets` = the probed hosts so far (20–64) |
| `flood.pcap` | 40 s baseline of 50 pps from 5 clients to `10.0.9.5:80`, then 5 s of 3000 pps from 60 sources in the documentation ranges | exactly 1 `flood` for `10.0.9.5`, `distinct_sources` ≥ 50; no `flood` during the baseline |
| `idle.pcap` | `10.0.1.30` is active 0–10 s, then silent; `10.0.1.10` sends 1 pkt/s until 140 s | exactly 1 `host_idle` for `10.0.1.30` at ts ≈ 130 s, with `10.0.1.30` in that tick's `removed_hosts` |
| `ipv6.pcap` | `fd00:1::10` → `2001:db8:ffff::1:443`, plus NDP | flows have IPv6 ids, `fd00:1::10` local, NDP traffic categorised as `control` |
| `vlan.pcap` | 802.1Q VLAN 10: `10.0.1.x`; VLAN 20: `10.0.9.x` | with `config/districts.example.yaml`: corp and core districts assigned by VLAN |
| `gateway.pcap` | 10 external IPs, all behind MAC `02:00:00:00:00:01` | `10.0.1.1` class `network`; no external host has a mac or vendor |
| `demo.pcap` | 120 s mixed traffic: 40 local hosts across corp and core with assorted vendors, about 200 external hosts, web/DNS/SSH/mDNS/SSDP, one SYN scan at t = 60 s | used by end-to-end tests and the README GIF; < 5 MB |
| `many_hosts.pcap` | 16 s: 1000 local clients in `10.0.16.0/22`, each exchanging a UDP datagram with its own server every 4 s (762 servers in the documentation ranges, 238 local in `10.0.30.0/24`): 2000 hosts | frame-time check (`web/e2e/perf.spec.ts`, T5.4); no golden file |

## Golden tests

- `internal/pipeline` exposes `Run(ctx, source, cfg, sink)`, used by both the binary and the tests.
- For each fixture, the golden test runs the pipeline with `--replay-speed 0` into a JSONL sink and compares it to `testdata/golden/<name>.jsonl`.
- `go test ./internal/pipeline -run Golden -update` rewrites the golden files. Golden updates must appear in the PR diff and be explained in its description.
- Determinism: `server.version` is replaced with `"test"`, the anonymisation key is fixed via a test hook, and arrays are sorted (PROTOCOL.md).
- The vlan and geo fixtures run with the example districts file and `test-city.mmdb` with `geo.home: [51.5, -0.12]`. Expected sectors: JP → NE, US → W, DE → E.

## Server and security tests (`internal/server`)

| Case | Expected |
|---|---|
| `/auth` with wrong or missing token | `401`, no cookie |
| `/auth` with correct token | `302` to `/`; cookie has `HttpOnly`, `SameSite=Strict`, `Path=/` |
| `/auth?next=//evil.example` | redirect goes to `/` |
| `/ws` without cookie | `401` |
| `/ws` with cookie and `Origin: http://evil.example` | `403` |
| `/ws` with cookie and allowed Origin | upgrade; first message is `hello` |
| `--listen 0.0.0.0:0` | WARN log line present |
| Slow client (never reads) | closed with 1013 within `client_queue` ticks; other clients keep receiving |
| Client sends 1 KB | connection closed |
| 5th client with `max_clients 4` | `503` |
| Every response | CSP, `nosniff`, `no-referrer` headers present |

## Frontend tests

Vitest covers the pure modules: `state/rate.ts` (EWMA, decay, height formula, with the numeric examples from VISUAL_SPEC), `layout/*.ts` (determinism, no overlaps for 2000 random ids, block probing, sector placement), `net/backoff.ts`, and protocol parsing.

Shading guards (VISUAL_SPEC, *Lighting, materials and shading*):
- **Vitest:** the material factories for buildings, plots and ground return `MeshStandardMaterial` with `wireframe === false`. The building mesh has `castShadow` and `receiveShadow`, and has a `customDepthMaterial`. The scene contains exactly one `HemisphereLight`, one shadow-casting `DirectionalLight` (when high quality) and a non-null `scene.environment`.
- **ESLint:** a `no-restricted-syntax` rule fails on a `wireframe: true` property, `new MeshBasicMaterial`, `new EdgesGeometry` and `new WireframeGeometry` anywhere in `web/src/render/` except the overlay effects `effects/searchlight.ts` (cone and target outlines) and `effects/meteor.ts` (shield).

Conformance: Go writes canonical example messages to `testdata/protocol/{hello,tick,events}.json` (generated by `go test ./internal/protocol -update`). The TS test parses each with the `protocol.ts` types and a runtime validator, and fails on a missing or extra required field.

## End-to-end (Playwright)

1. `make build`, then start `bin/cybercity --read testdata/pcap/demo.pcap --replay-speed 4 --loop --listen 127.0.0.1:0 --url-file <tmp>`.
2. Open `<url>&next=%2F%3Fdebug%3D1`, wait for `window.__cybercity.ready` (timeout 20 s).
3. Assert:
   - `stats().buildings ≥ 30`
   - `stats().highways ≥ 5`
   - `stats().particles > 0`
   - a `port_scan` event in `lastEvents()` within 20 s
   - an event log entry is visible
4. Pass even under software rendering: headless Chromium uses SwiftShader, so assert that the software warning **is** shown and quality is `low`.
5. **Shading test** (`web/e2e/shading.spec.ts`), at viewport 1280×720 and device scale factor 1:
   1. Open with `next=%2F%3Fdebug%3D1%26quality%3Dhigh%26pose%3Dauto`. Wait until `poseHost()` is non-null (timeout 20 s), then 2 s more for the camera and height to settle. Use `id = poseHost()`.
   2. Take a screenshot and decode it with `pngjs`.
   3. Shrink each polygon from `probe(id)` by 15% toward its centroid.
   4. Assert:
      - `stats().wireframe === false`, `stats().lit === true`, `stats().shadows === true`.
      - **Filled:** in each face polygon, ≥ 90% of pixels differ from `bg` by relative luminance > 0.02. A wireframe or edges-only body scores far below this.
      - **Lit:** the facade body of the `px` face (moon-lit) is brighter than the `pz` face (sky-lit) by ≥ 8%, measured at the 30th luminance percentile of each face. This excludes the emissive windows, which are identical on both faces and would dominate a mean.
      - **Shadowed:** around the building (`probe(id).ring`, 5×5 px samples, face pixels excluded), the darkest ground in the sector away from the moon (112°–202°) is ≥ 5% darker than the median of the moon-facing, camera-visible ground (300°–90°). The cast shadow is a narrow wedge, which a single fixed sample point tends to miss. The page is opened with `hide=traffic`, so highways and particles do not cover the ground. `pose=auto` guarantees the host is local, ≥ 10 tall and has `sameSurface === true`. Both points were checked geometrically to be visible from the pose, with only the shadow point in shadow, for heights 10–40.
   5. Save the crop as `web/e2e/artifacts/shading.png`.
6. Save a screenshot to `web/e2e/artifacts/city.png`, uploaded as a CI artifact. This is how a reviewer, human or agent, sees the city.
7. Negative test: opening `/` without auth shows `Invalid link`.

## CI (`.github/workflows/ci.yml`)

Runs on pull requests and pushes to `main`. Jobs:
- **go**: on ubuntu-latest, `apt-get install libpcap-dev`, then `go vet`, `golangci-lint`, `go test -race ./...`, `govulncheck`, and the fixturegen diff check.
- **web**: `npm ci`, `tsc --noEmit`, `eslint`, `vitest run`, `vite build`.
- **e2e**: needs both jobs; builds the binary with the web bundle, then runs Playwright and uploads the screenshot.
- **live-smoke**: runs `sudo go test -tags live ./internal/capture` on the `lo` interface.
- **release-dry-run**: `goreleaser release --snapshot --clean` on amd64 (T5.2).
- **perf** (non-blocking): see *Performance* below.

A task counts as done only when all jobs are green.

## Performance

- **Throughput:** `go test -run XXX -bench BenchmarkReplay ./internal/pipeline` replays `demo.pcap` from memory through the whole pipeline (decode, enrichment, detectors, windows, JSON encoding) as fast as possible, without `-race`. It reports `pkts/s`; the target is ≥ 200k.
- **Frame time:** `web/e2e/perf.spec.ts` starts its own server on `many_hosts.pcap` (2000 hosts), samples `requestAnimationFrame` deltas for 5 s, and writes p50/p95 to `web/e2e/artifacts/perf.json`. Under headless SwiftShader nearly all of the frame is software rasterisation, so these numbers track regressions, not real GPU frame rates.
- **Baseline:** `testdata/perf/baseline.json`. The `perf` CI job runs `tools/perfcheck`, which fails its step (an error annotation, not the workflow) when a metric is more than 20% worse. After an intended change, update the baseline from the job summary.
