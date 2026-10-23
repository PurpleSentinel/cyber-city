# AGENTS.md

Instructions for coding agents (and humans) working on CyberCity-3D: a Go service that passively captures network metadata and streams it to a three.js "cyber city" in the browser.

## Read first, in this order
1. `docs/TASKS.md`: pick the first unticked task whose dependencies are done.
2. The spec sections that task cites: `docs/PROTOCOL.md`, `docs/CONFIG.md`, `docs/VISUAL_SPEC.md`, `docs/TESTING.md`.
3. `docs/DECISIONS.md`: settled choices; do not re-open them.
4. `docs/DESIGN.md` for intent, `ROADMAP.md` for the big picture.

The specs are normative. If code and spec disagree, fix the code, or change the spec in the same PR with a reason.

## Repository layout (target; created by T0.2)
```
cmd/cybercity/          main: flags → config → capture → privdrop → server
internal/config/        defaults, YAML, flags, validation
internal/capture/       live (pcap, cgo) and file (pcapgo) sources; interface discovery
internal/pipeline/      decode → flows → windows → protocol messages; Run() used by binary and golden tests
internal/hosts/         host table, change tracking, eviction
internal/enrich/        services, RTT, retransmits, OUI/vendor, gateway, device class, geo
internal/districts/     district matching and auto districts
internal/detect/        scan, flood, idle detectors
internal/protocol/      wire types (mirror of docs/PROTOCOL.md) and ordering helpers
internal/server/        HTTP, auth, static files, WebSocket hub
internal/privdrop/      privilege dropping
web/                    Vite + TypeScript frontend; embed.go embeds web/dist
  src/config.ts         every VISUAL_SPEC constant
  src/protocol.ts       wire types (mirror of internal/protocol)
  src/net/              session check, WebSocket, backoff
  src/state/            store, rates
  src/layout/           deterministic placement
  src/render/           scene, buildings, highways, particles, post, effects/
  src/ui/               hud, inspector, eventlog, styles.css
  test/                 Vitest
  e2e/                  Playwright
tools/fixturegen/       synthetic pcaps + test mmdb → testdata/
tools/ouigen/           IEEE OUI CSV → internal/enrich/oui_table.go
testdata/               pcap/, golden/, protocol/, geo/
config/                 example config and districts files
```

## Commands
Run everything through `make` so local runs and CI match.

| Target | Does |
|---|---|
| `make help` | List targets |
| `make setup` | `go mod download`, `npm ci` in `web/`, `npx playwright install --with-deps chromium` |
| `make fixtures` | Regenerate `testdata/` with fixturegen |
| `make oui` | Regenerate the OUI table (network access needed) |
| `make web` | `vite build` → `web/dist` |
| `make build` | `make web`, then `go build -o bin/cybercity ./cmd/cybercity` |
| `make test` | `go test -race ./...` and `vitest run` |
| `make lint` | `golangci-lint run`, `tsc --noEmit`, `eslint` |
| `make e2e` | `make build`, then Playwright |
| `make check` | lint + test + fixture-diff check: run before every push |
| `make dev` | Backend replaying `demo.pcap` plus the Vite dev server (prints the dev URL) |
| `make demo` | `make build`, then `bin/cybercity --read testdata/pcap/demo.pcap --loop` |
| `make setcap` | `sudo setcap cap_net_raw,cap_net_admin+eip bin/cybercity` (live capture without root) |

### Environment
- Go ≥ 1.24, Node 22 (`.nvmrc`), and libpcap headers for live capture: Fedora `sudo dnf install golang libpcap-devel nodejs`, Debian/Ubuntu `sudo apt install golang libpcap-dev nodejs npm`.
- If a tool is missing locally, say so in the PR and rely on CI. Don't skip checks silently.
- Tests never need root, network access or a real interface, except the `live`-tagged smoke test.

## Workflow
- One task (TASKS.md) per branch and PR: branch `task/<id>-<slug>`, PR title `<id>: <title>`.
- Keep PRs small. If a task grows beyond about 600 changed lines (excluding generated and golden files), split it and add the sub-tasks to TASKS.md.
- In the PR: tick the task box, list the spec sections implemented, paste the "Done when" evidence (test names, numbers), explain any golden-file changes, and add DECISIONS.md entries for new choices.
- Don't start a task whose dependencies aren't merged.
- Never push to `main`, force-push shared branches, or merge your own PR unless the owner asked for it.

## Hard rules (security and privacy)
1. Default bind is `127.0.0.1`. Never change the default to a wildcard address.
2. `/ws` and `/api/*` always require the session cookie and an allowed `Origin`. Never add a bypass, debug or otherwise.
3. Never parse, store, log or send packet payloads. Snaplen stays small (D8).
4. Never commit real packet captures, real MAC/IP inventories or GeoLite databases. Fixtures come from fixturegen, with documentation address ranges only.
5. No outbound network calls at runtime (no telemetry, no online lookups). Reverse DNS, if ever added, is opt-in and off by default.
6. Capture privileges come from capabilities or are dropped after opening the handle. Never require running the server as root.
7. No inline scripts or styles in the frontend (CSP).
8. The debug hook is read-only.
9. Detections are labelled heuristic. Never describe this project as an IDS or claim to detect malware.

## Conventions
**Go**
- `gofmt`, golangci-lint clean.
- Errors wrapped with `%w`; `log/slog` for logging.
- No package-level mutable state; dependencies passed explicitly.
- `context.Context` for shutdown; every goroutine has an owner that waits for it.
- Table-driven tests.
- Never range over a map into output without sorting (D16).

**TypeScript**
- `strict`, no `any`, no non-null assertions without a comment.
- Constants come from `config.ts`.
- Pure logic (rates, layout, formulas) lives in modules without three.js imports, so Vitest can test it.

**three.js**
- The city is solid, lit and shadowed (VISUAL_SPEC *Lighting, materials and shading*, D17). Never use `wireframe: true`, `MeshBasicMaterial` or edge/wireframe geometry for city objects; ESLint and the e2e shading test enforce this.
- No allocations in the render loop: reuse scratch `Object3D`/`Matrix4`/`Color`.
- Use instancing for anything that repeats.
- `dispose()` geometries and materials you replace.
- Time-based animation using frame dt, never per-frame constants.

**Protocol changes**
- Update `docs/PROTOCOL.md`, `internal/protocol` and `web/src/protocol.ts` together, then regenerate `testdata/protocol`.

**Comments**
- Explain *why*. Reference the spec section a non-obvious constant comes from.

## When the spec is unclear
Pick the simplest option consistent with DESIGN.md and the hard rules, implement it, and record it in DECISIONS.md in the same PR. If the choice is a product decision (scope, licensing, anything marked 👤), stop and ask the owner in the PR instead.
