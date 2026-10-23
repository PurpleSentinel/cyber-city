# Visual Specification

Exact values for the city renderer. All of them live in `web/src/config.ts`; renderer code imports from there and never hard-codes them. Units: 1 world unit ≈ 1 m; time in seconds unless noted. The concept behind each element is in DESIGN.md.

## Palette

| Token | Hex | Use |
|---|---|---|
| `bg` | `#05050a` | Clear colour and fog |
| `gridMajor` / `gridMinor` | `#00f0ff` / `#052535` | Ground grid |
| `hud` | `#00f0ff` | HUD text and glow |
| `alertSky` | `#2a0008` | Meteor-shower sky |
| `alert` | `#ff2040` | Searchlight, scan target outlines |

Device class (building body):

| Class | Hex |
|---|---|
| `workstation` | `#3d7cff` |
| `server` | `#22e07a` |
| `iot` | `#ff3df2` (magenta; DECISIONS D11) |
| `network` | `#ffb020` |
| `unknown` | `#00f0ff` (the default holographic cyan) |

Traffic category (particles, highway tint, facade windows):

| Category | Hex | Window behaviour |
|---|---|---|
| `https` | `#22e07a` | steady |
| `web_cleartext` | `#ff8c1a` | steady |
| `dns` | `#ffd23f` | steady |
| `ssh` | `#ffffff` | blinks at 2 Hz (50% duty) |
| `legacy_cleartext` | `#ff2040` | steady |
| `discovery` | `#a970ff` | steady |
| `control` | `#7f9cbf` | (not a service) |
| `other` | `#8a8fa3` | steady |

## Scene and camera

- `PerspectiveCamera(fov 60, near 0.5, far 2000)`, start position `(170, 140, 170)`, looking at `(0, 0, 0)`.
- `OrbitControls`: damping 0.08, `maxPolarAngle` 85°, distance 40–700.
- `FogExp2(bg, 0.0022)`.
- Renderer: `antialias: true`, `powerPreference: "high-performance"`, pixel ratio `min(devicePixelRatio, 2)` (forced to 1 in low quality), `toneMapping: ACESFilmicToneMapping`, `toneMappingExposure: 1.0`, `outputColorSpace: SRGBColorSpace`, `shadowMap.enabled = true` (high quality), `shadowMap.type = PCFShadowMap` (three.js r18x removed `PCFSoftShadowMap`).
- Post-processing chain: `RenderPass` → `UnrealBloomPass(strength 0.9, radius 0.4, threshold 0.85)` → `OutputPass`, which applies tone mapping and sRGB. The threshold is high so that only emissive windows, edges and effects bloom, not the lit building bodies.

## Lighting, materials and shading

**Rule: the city is solid and lit, never wireframe.** Buildings, district plots and the ground use filled, lit `MeshStandardMaterial`s that receive light and shadow. The following are not allowed for city geometry:
- `wireframe: true`
- `MeshBasicMaterial`
- `LineSegments`/`EdgesGeometry` as a building body

Lines are allowed only for the ground grid overlay and the meteor-shower shield effect. Low quality and software rendering keep lit materials; they drop only shadows, bloom and particle count.

**Light rig**

| Light | Settings |
|---|---|
| `HemisphereLight` | sky `#1a2a4a`, ground `#05050a`, intensity 0.6 |
| `DirectionalLight` ("moon") | colour `#9fb8ff`, intensity 1.4. It shines from direction `(0.7, 1.0, −0.3)` (normalised): the light sits at that direction × 400 and targets the origin. +x faces are lit directly while +z faces get only sky and environment light, so adjacent faces always read differently. Shadows fall toward (−0.7, +0.3): about 0.76 × building height long, and visible from the default camera. `castShadow` in high quality: map size 2048², orthographic shadow camera ±300, near 10, far 900, `bias −0.0005`, `normalBias 0.5`. |
| Environment | `scene.environment = PMREMGenerator.fromScene(new RoomEnvironment(), 0.04).texture`, environment intensity 0.35. Procedural, with no external HDR files. It gives the glass bodies soft reflections. |

**Ground**
- `PlaneGeometry(640, 640)` laid flat, `MeshStandardMaterial(color #080a12, roughness 0.55, metalness 0.3)`, `receiveShadow`.
- The grid overlay sits at y = 0.02 with `depthWrite: false`: major lines in `gridMajor` at 35% opacity, minor lines in `gridMinor` at 20%.
- Each drawn local district has a plot slab of 64 × 0.2 × 64 (top at y = 0.2, buildings stand on it) using `MeshStandardMaterial(color #0b0e1a, roughness 0.6, metalness 0.2)`, `receiveShadow`, with a 0.3-wide emissive border strip in `hud` at intensity 0.4.

**Building material.** One `MeshStandardMaterial`, extended through `onBeforeCompile`; never a `ShaderMaterial`, which would lose three.js lighting. Shaded "dark glass" tinted by device class:
- **Albedo** `= mix(#0c1020, classColor, 0.25)`, roughness 0.35, metalness 0.6, `side: FrontSide`.
- **Emissive terms**, added in the fragment shader on top of the lit result:
  - **Windows** (facade spec below): category colour × 1.6. This is above the bloom threshold, so windows glow.
  - **Edge accents:** a 0.06-unit band along the 4 vertical edges and the roof perimeter, in `classColor` × 1.2. An accent on a solid body, not a wireframe.
  - **Fresnel rim:** `classColor × 0.35 × pow(1 − max(dot(N, V), 0), 3)`. Keeps the holographic feel.
  - **Roof cap:** `classColor × 0.25` on the top face, so roofs read from above.
- The mesh has `castShadow` and `receiveShadow` on. A `customDepthMaterial` (`MeshDepthMaterial` with the same `onBeforeCompile` dissolve discard and instance attributes) keeps shadows consistent while buildings dissolve or fade.

## Layout

Everything is deterministic: positions depend only on ids, never on random numbers or arrival order (except where probing resolves a collision). The hash is `fnv1a32(utf8(id))`.

**Local districts** (kinds `lan` and `server`):
- Up to 9 local districts sit on a 3 × 3 grid of blocks. A block is 64 × 64, avenues between blocks are 16 wide, so the grid spans −112…+112 on x and z.
- Block index:
  - `server` districts probe from block 4 (the centre) in the order 4, 1, 3, 5, 7, 0, 2, 6, 8.
  - `lan` districts probe in the order `hash(districtId) mod 9`, then +1 mod 9, ….
  - Taken blocks are skipped.
  - A 10th or later local district is not drawn; the HUD shows `+N districts hidden`.
- Inside a block there are 16 × 16 slots at a pitch of 4; a building footprint is 2 × 2, centred on its slot. Slot = `hash(hostId) mod 256`, with linear probing to the next free slot. Overflow hosts are not drawn; they count in the HUD's `hidden` total.
- District label: a flat text sprite at the block's −z edge, colour `hud`, opacity 0.6.

**The Outlands** (non-local hosts):
- 11 rings at radius `r = 180, 188, …, 260`.
- Ring `i` has `floor(2π·r / 4)` angular slots, split evenly over 8 sectors of 45° each. Sector order is N, NE, E, SE, S, SW, W, NW; N is the −z direction and angles increase clockwise when viewed from above.
- Sector = `host.geo.sector` if present, else `hash(first two octets, or first 32 bits for IPv6)` mod 8. With `--anonymize` the frontend only has the id, so the sector is `hash(hostId) mod 8`.
- Slot = `hash(hostId)` mod (slots in that sector across all rings), linear probing.

## Buildings

- One `InstancedMesh` of `BoxGeometry(2, 1, 2)` translated so y ∈ [0, 1], capacity `max_hosts` (2000).
- Material: the lit building material above, reading these per-instance attributes:
  - `aColor` (vec3): device-class colour.
  - `aServices` (float, 0–255): category bitmask from the host's `services` (bit order in CONFIG.md).
  - `aState` (vec4): `x` degraded 0–1, `y` flash 0–1, `z` dissolve 0–1, `w` fade-in 0–1.
- Body: solid and lit, as defined in *Lighting, materials and shading*. Buildings stand on their district plot (y = 0.2) or on the ground in the Outlands.
- **Facade windows**: on each of the 4 side faces, the face is split into 6 vertical columns, assigned to the set bits of `aServices` in bit order, repeating if fewer than 6. Within a column, windows are 0.25 wide × 0.5 tall at 1.0 vertical pitch, coloured by category, emissive. The `ssh` column blinks at 2 Hz. A host with no services has dim cyan windows on alternate floors at 15% intensity.

### Height

- **Rate per host** (bytes/s), updated in the frontend from flows (a flow adds its bytes to both endpoints):
  - On each tick, `sample = bytes_in_tick / (window_ms/1000)`, `alpha = 1 − 2^(−Δt / 2.0)`, `rate = rate + alpha·(sample − rate)`, where Δt = tick.ts − host.lastSampleTs in seconds.
  - Between ticks the displayed rate decays as `rate · 2^(−(now − lastSampleTs) / 2.0)`, so buildings sink even if ticks stop.
- **Target height** `= min(40, 1 + 6·log10(1 + rate / 1000))`. That gives about 2.8 at 1 kB/s, 19 at 1 MB/s and 31 at 100 MB/s.
- **Displayed height** follows the target with a time constant τ = 0.25 s: `h += (target − h)·(1 − e^(−dt/τ))`. This uses frame dt, never a fixed per-frame factor.

### Status effects

| Effect | Trigger | Look |
|---|---|---|
| Degraded | `rtt_ms > 150` or `retrans_rate > 0.02`. `degraded = clamp(max((rtt_ms−150)/350, (retrans_rate−0.02)/0.08), 0, 1)` | Horizontal scanline noise and UV jitter of up to 0.15 units; colour shifted toward magenta by `degraded·0.5` |
| Spike flash | Instantaneous sample > 4 × rate and sample > 1 MB/s | A white band rises from the ground to the roof over 0.6 s (`aState.y` 1 → 0) |
| Lightning | Sample > 10 × rate and > 10 MB/s, or the host is a `port_scan` target | A jagged additive line from y = 200 to the roof: 3 segments re-randomised every 50 ms, lasting 0.4 s, colour `#e0f7ff` |
| Fade-in | New host | `aState.w` 0 → 1 over 0.8 s; scale y grows from 0 |
| Removed (no event) | Host in `removed_hosts` without `host_idle` | Fades out over 1.0 s |

## Highways

- One road per host pair with traffic in the last 3 s. At most 500, chosen by rate; the rest are not drawn.
- Route:
  - local ↔ local: an L-shaped path (x first, then z) between building centres, 0.1 above the plot tops (y = 0.3).
  - Anything involving the Outlands: a straight line, at the same height.
- Width `w = clamp(0.2 + 0.6·log10(1 + pairRate/1000), 0.2, 3)`. Rendered as instanced flat quads, additive blending, colour = the dominant category of the pair over the last 3 s, opacity 0.35.
- After the last packet, opacity decays to 0 over 3 s, then the road is freed.

## Particle freight

- Instanced small boxes (0.3 × 0.3 × 0.6) oriented along the road, additive, category colour. Pool cap 5000 (2500 in low quality); when full, new particles are skipped.
- Emission per road: `clamp(1 + log2(1 + pps), 1, 20)` particles/s, where pps is the pair's packets/s. Direction follows the flow (both directions when traffic flows both ways).
- Travel time along the road: `clamp(0.4 + rtt_ms·0.02, 0.4, 4.0)` s, or 1.5 s if the RTT is unknown.

## Event effects

| Event | Duration | Spec |
|---|---|---|
| `port_scan` (Searchlight) | 6 s | An additive `ConeGeometry(radius 12, height 60)` in `alert` at opacity 0.22, with its apex on top of the `src` building. It aims at each target in turn for 0.3 s each (looping if time remains). Targets get an `alert` outline for the duration. Scan targets also receive lightning. |
| `flood` (Meteor Shower) | 8 s | Fog and clear colour lerp to `alertSky` over 1 s, hold, and return over 2 s. 400 particles (200 in low quality) start at y = 200 within a 120-unit radius of the target and fall onto it (speed 60–120 units/s). A wireframe `IcosahedronGeometry(radius = max(6, height·0.6), 1)` shield around the target flickers its opacity randomly between 0.1 and 0.6 at 12 Hz. |
| `host_idle` (Power Outage) | 5 s | Windows flicker on/off at random (10 Hz) for 2 s, then the building dissolves over 3 s: noise-threshold discard driven by `aState.z`, while scale y drops to 0. The instance is then freed. |

If several events hit the same building, the newest wins. At most 3 searchlights and 2 meteor showers run at once; extra events are only logged.

## HUD and interaction

- **Top-left panel**:
  - Title.
  - Connection state: `Connecting…`, `Live`, `Reconnecting in Ns`, `Paused` or `Invalid link`.
  - Source (`live: enp3s0` or `file`).
  - Packets/s and bytes/s, as a 1 s moving average.
  - Hosts as `shown / total (+hidden)`.
  - Kernel drops, shown in `alert` colour when > 0.
  - Renderer name.
- **Software rendering warning**:
  - If `WEBGL_debug_renderer_info` reports a renderer matching `/llvmpipe|swiftshader|software/i`, show `⚠ Software rendering — enable GPU acceleration` and switch to low quality automatically.
  - If the extension is unavailable, show `Renderer: unknown`.
- **Inspector**: clicking a building opens a right-hand panel showing id/IP, vendor, device class ("heuristic"), district, services (port, category, confirmed), bytes/s, RTT, retransmission %, first and last seen. Hovering shows a tooltip with id and vendor. Picking uses `Raycaster` against the `InstancedMesh` (`instanceId`).
- **Event log**, bottom-right: the last 50 events, newest first: time, kind, `src → targets` (first 3 plus `+N`), and an evidence summary. Footer: `Heuristic detections — not an IDS`.
- **Keys**:
  - `Space`: pause; ticks are dropped while paused and the display freezes.
  - `L`: toggle low quality (no bloom, no shadows, half particles, pixel ratio 1; materials stay lit).
  - `F`: fly to the selected building.
  - `Esc`: clear the selection.
  - `?`: help overlay.
- **Reconnect**: backoff of 0.5 s × 2ⁿ, capped at 10 s, with ±20% jitter. Before each attempt, call `/api/session`; on `401`, show `Invalid link` and stop.
- **Styles**: in `web/src/ui/styles.css` only. No inline styles or scripts (because of the CSP).

## Debug hook (for tests)

When the page URL has `?debug=1`, expose a read-only `window.__cybercity`:
```ts
{
  ready: boolean,                  // true after the first hello has been applied and a frame rendered
  stats(): { buildings: number; highways: number; particles: number; ticks: number;
             events: number; renderer: string; software: boolean; quality: "high" | "low";
             lit: boolean;        // every city material is a lit MeshStandardMaterial
             wireframe: boolean;  // true if any city material has wireframe enabled (must be false)
             shadows: boolean },
  hostIds(): string[],
  lastEvents(): { id: string; kind: string }[],
  // Screen-space (CSS px) points for shading tests:
  //   px, pz: corners of the building's +x and +z side faces
  //   shadow: ground point at centre + (−0.35h, +0.15h), inside the cast shadow
  //   lit:    ground point at centre + (+0.15h, +0.35h), same distance, outside the shadow
  //   sameSurface: both ground points lie on the same district plot (or both on bare ground)
  probe(hostId: string): { px: [number, number][]; pz: [number, number][];
                           shadow: [number, number]; lit: [number, number];
                           sameSurface: boolean } | null,
  poseHost(): string | null,   // the host the camera is locked on (pose=…), once chosen
}
```
Extra debug-only URL parameters (they only change the view, never the data):
- `quality=high|low` forces the quality mode, including under software rendering.
- `pose=auto` waits until 10 s of tick time have been applied, then locks onto the busiest local host whose displayed height is ≥ 10 and whose `probe` has `sameSurface === true`, re-checking each second until one qualifies.
- `pose=host:<id>` fixes the camera at `(x + d, 0.6·h + 6, z + d)` with `d = max(14, 0.9·h)`, where `(x, z)` is the building's position and `h` its displayed height. It looks at the building's mid-height and disables orbit input. The +x and +z faces are then visible and lit differently by the moon.
It is never a write path. Tests reach it through the `/auth` redirect: `/auth?token=…&next=%2F%3Fdebug%3D1` (PROTOCOL.md).

## Performance budget

At 2000 buildings, 500 highways and 5000 particles with shadows on: 60 fps on a mid-range integrated GPU. At most 4 draw calls for the city layers (plots, buildings, highways, particles) plus 1 shadow-pass draw for buildings, excluding post-processing. No per-frame allocations in the render loop. `Object3D`, `Matrix4` and `Color` scratch objects are reused.
