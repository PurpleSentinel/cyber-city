import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { RoomEnvironment } from "three/examples/jsm/environments/RoomEnvironment.js";
import { CAMERA, LIGHTING, PALETTE, SCENE } from "./config";
import { debugEnabled, installDebug } from "./debug";
import { Layout } from "./layout/layout";
import type { ConnectionState } from "./net/connection";
import { Connection, defaultDeps } from "./net/connection";
import { INVALID_LINK_MESSAGE } from "./net/session";
import type { Host } from "./protocol";
import { Buildings } from "./render/buildings";
import { Lightning } from "./render/effects/lightning";
import { Highways } from "./render/highways";
import { Particles } from "./render/particles";
import { poseFor, probe } from "./render/probe";
import { rendererInfo } from "./render/rendererInfo";
import { QualityController, createComposer } from "./render/quality";
import { Plots, addLights, configureRenderer, createGround, setEnvironment } from "./render/scene";
import { HighwayModel } from "./state/highways";
import { bytesByHost } from "./state/rate";
import { Store } from "./state/store";
import { HELP_TEXT, Hud, MovingRate, formatHosts, formatPps, formatRate } from "./ui/hud";
import { InspectorView, inspectorRows, tooltipText } from "./ui/inspector";
import "./ui/styles.css";

// App wiring: the shaded city of docs/VISUAL_SPEC.md.

const params = new URLSearchParams(window.location.search);

// ---- Renderer, scene, camera ------------------------------------------------
const renderer = new THREE.WebGLRenderer({ antialias: true, powerPreference: "high-performance" });
const gpu = rendererInfo(renderer.getContext());
const forced = params.get("quality");
const quality: "high" | "low" = forced === "high" || forced === "low" ? forced : gpu.software ? "low" : "high";
renderer.setPixelRatio(quality === "low" ? 1 : Math.min(window.devicePixelRatio, SCENE.maxPixelRatio));
renderer.setSize(window.innerWidth, window.innerHeight);
renderer.setClearColor(PALETTE.bg);
configureRenderer(renderer, quality);
document.body.appendChild(renderer.domElement);

const scene = new THREE.Scene();
scene.fog = new THREE.FogExp2(PALETTE.bg, SCENE.fogDensity);
const camera = new THREE.PerspectiveCamera(CAMERA.fov, window.innerWidth / window.innerHeight, CAMERA.near, CAMERA.far);
camera.position.set(...CAMERA.position);
const controls = new OrbitControls(camera, renderer.domElement);
controls.enableDamping = true;
controls.dampingFactor = CAMERA.damping;
controls.maxPolarAngle = (CAMERA.maxPolarAngleDeg * Math.PI) / 180;
controls.minDistance = CAMERA.minDistance;
controls.maxDistance = CAMERA.maxDistance;

const { moon } = addLights(scene, quality);
const pmrem = new THREE.PMREMGenerator(renderer);
setEnvironment(scene, pmrem.fromScene(new RoomEnvironment(), LIGHTING.environment.sigma).texture);
pmrem.dispose();
const ground = createGround();
scene.add(ground);
const plots = new Plots();
scene.add(plots.mesh);
const buildings = new Buildings();
scene.add(buildings.mesh);
const lightning = new Lightning();
scene.add(lightning.group);
const highwayModel = new HighwayModel();
const highways = new Highways();
scene.add(highways.mesh);
const particles = new Particles(quality === "low");
scene.add(particles.mesh);

// ---- Post-processing and quality (key L) ---------------------------------------
const { composer, bloom } = createComposer(renderer, scene, camera);
const groundMaterials: THREE.Material[] = [];
ground.traverse((o) => {
  if (o instanceof THREE.Mesh && o.material instanceof THREE.MeshStandardMaterial) groundMaterials.push(o.material);
});
const qualityCtl = new QualityController(
  {
    renderer,
    moon,
    materials: [buildings.mesh.material as THREE.Material, plots.mesh.material as THREE.Material, ...groundMaterials],
    particles: particles.system,
    bloom,
    devicePixelRatio: window.devicePixelRatio,
  },
  quality,
);
// ---- Inspector: hover tooltip, click panel, F fly-to, Esc clear ----------------
const inspector = new InspectorView();
const raycaster = new THREE.Raycaster();
const pointer = new THREE.Vector2();
let inspectedAt = 0;
let flight: { from: THREE.Vector3; to: THREE.Vector3; fromT: THREE.Vector3; toT: THREE.Vector3; t: number } | null = null;

function pick(clientX: number, clientY: number): string | null {
  pointer.set((clientX / window.innerWidth) * 2 - 1, -(clientY / window.innerHeight) * 2 + 1);
  raycaster.setFromCamera(pointer, camera);
  buildings.mesh.computeBoundingSphere();
  const hit = raycaster.intersectObject(buildings.mesh, false)[0];
  return hit?.instanceId === undefined ? null : buildings.idAt(hit.instanceId);
}

function showInspector(id: string): void {
  const h = store.hosts.get(id);
  if (!h) return;
  const district = store.districts.get(h.district)?.name ?? h.district;
  inspector.show(id, inspectorRows(h, district, buildings.rateOf(id)));
}

renderer.domElement.addEventListener("pointermove", (e) => {
  const id = pick(e.clientX, e.clientY);
  const h = id ? store.hosts.get(id) : undefined;
  inspector.hover(h ? tooltipText(h) : null, e.clientX, e.clientY);
});
renderer.domElement.addEventListener("click", (e) => {
  const id = pick(e.clientX, e.clientY);
  if (id) showInspector(id);
});

function flyToSelected(): void {
  const g = inspector.selected ? buildings.geometry(inspector.selected) : null;
  if (!g) return;
  const { position, target } = poseFor(g);
  flight = { from: camera.position.clone(), to: position, fromT: controls.target.clone(), toT: target, t: 0 };
}

function stepFlight(dt: number): void {
  if (!flight) return;
  flight.t = Math.min(1, flight.t + dt); // one second
  const k = flight.t * flight.t * (3 - 2 * flight.t); // smoothstep
  camera.position.lerpVectors(flight.from, flight.to, k);
  controls.target.lerpVectors(flight.fromT, flight.toT, k);
  if (flight.t >= 1) flight = null;
}

// Keys (VISUAL_SPEC "HUD and interaction").
const helpBox = document.getElementById("help");
const helpList = document.getElementById("help-list");
for (const line of HELP_TEXT) {
  const li = document.createElement("li");
  li.textContent = line;
  helpList?.appendChild(li);
}
window.addEventListener("keydown", (e) => {
  if (e.ctrlKey || e.metaKey || e.altKey) return;
  if (e.key === "l" || e.key === "L") {
    qualityCtl.toggle();
    composer.setPixelRatio(renderer.getPixelRatio());
  } else if (e.key === " ") {
    e.preventDefault();
    paused = !paused;
    if (!paused) connection.resync(); // dropped ticks: rebuild from a fresh hello
    hud.set({ status: describe(lastState) });
  } else if (e.key === "?") {
    if (helpBox) helpBox.hidden = !helpBox.hidden;
  } else if (e.key === "f" || e.key === "F") {
    flyToSelected();
  } else if (e.key === "Escape") {
    inspector.clear();
  }
});

function endpoint(id: string) {
  const g = buildings.geometry(id);
  return g ? { point: { x: g.x, z: g.z }, local: g.local } : null;
}
const layout = new Layout();

// ---- State → city -------------------------------------------------------------
const store = new Store();
let firstTickTs = 0;
let lastTickTs = 0;

function place(h: Host): void {
  const kind = store.districts.get(h.district)?.kind ?? (h.local ? "lan" : "outlands");
  const p = layout.place(h.id, h.district, kind, h.geo?.sector ?? null);
  if (!p) return;
  plots.update(layout.blocks());
  buildings.add(h, p, kind !== "outlands");
}

store.subscribe({
  reset() {
    buildings.clear();
    highwayModel.clear();
    particles.clear();
    layout.reset();
    plots.update([]);
    firstTickTs = 0;
  },
  hostAdded: place,
  hostUpdated: (h) => buildings.update(h),
  hostRemoved(id) {
    if (inspector.selected === id) inspector.clear();
    buildings.remove(id);
    highwayModel.removeHost(id);
  },
  tick(t) {
    if (firstTickTs === 0) firstTickTs = t.ts;
    lastTickTs = t.ts;
    highwayModel.applyTick(t.flows, t.window_ms, t.ts, (id) => buildings.has(id));
    const roads = highwayModel.roads(t.ts);
    highways.update(roads, endpoint);
    particles.setRoads(roads, endpoint);
    for (const id of buildings.applyBytes(bytesByHost(t.flows), t.window_ms, t.ts)) {
      const g = buildings.geometry(id);
      if (g) lightning.strike(g.x, g.z, g.base + g.height);
    }
  },
});

// ---- Fixed camera poses for tests (?pose=auto | host:<id>) -------------------
const poseParam = params.get("pose");
let poseHost: string | null = poseParam?.startsWith("host:") ? poseParam.slice(5) : null;

function blockCentres() {
  return layout.blocks().map((b) => b.centre);
}

function choosePoseHost(): string | null {
  if (lastTickTs - firstTickTs < 10_000) return null; // 10 s of tick time
  let best: string | null = null;
  let bestRate = -1;
  for (const id of buildings.ids()) {
    const g = buildings.geometry(id);
    if (!g || !g.local || g.height < 10) continue;
    const pr = probe(g, camera, window.innerWidth, window.innerHeight, blockCentres());
    const rate = buildings.rateOf(id);
    if (pr.sameSurface && rate > bestRate) {
      best = id;
      bestRate = rate;
    }
  }
  return best;
}

function applyPose(): void {
  if (poseParam === "auto" && !poseHost) poseHost = choosePoseHost();
  if (!poseHost) return;
  const g = buildings.geometry(poseHost);
  if (!g) return;
  const { position, target } = poseFor(g);
  camera.position.copy(position);
  controls.target.copy(target);
  controls.enabled = false;
}

// ---- HUD -------------------------------------------------------------------
const hud = new Hud();
const traffic = new MovingRate();
const rendererDiv = document.getElementById("renderer");
const warningDiv = document.getElementById("warning");
const setText = (el: HTMLElement | null, text: string): void => {
  if (el) el.textContent = text;
};
setText(rendererDiv, `Renderer: ${gpu.name}`);
if (gpu.software && warningDiv) {
  warningDiv.hidden = false;
  setText(warningDiv, "⚠ Software rendering — enable GPU acceleration");
}

let paused = false;
let lastState: ConnectionState = { kind: "connecting" };

function describe(s: ConnectionState): string {
  if (paused) return "Paused";
  switch (s.kind) {
    case "connecting":
      return "Connecting…";
    case "live":
      return "Live";
    case "reconnecting":
      return `Reconnecting in ${Math.ceil(s.inMs / 1000)}s`;
    case "invalid":
      return `Invalid link — ${INVALID_LINK_MESSAGE}`;
  }
}

// ---- Debug hook --------------------------------------------------------------
const debug = debugEnabled()
  ? installDebug({
      stats: () => ({
        buildings: buildings.size,
        highways: highways.roadCount,
        particles: particles.system.live.length,
        ticks: store.ticks,
        events: store.events.length,
        renderer: gpu.name,
        software: gpu.software,
        quality: qualityCtl.mode,
        lit: buildings.mesh.material instanceof THREE.MeshStandardMaterial,
        wireframe: (buildings.mesh.material as THREE.MeshStandardMaterial).wireframe,
        shadows: qualityCtl.shadows && buildings.mesh.castShadow,
      }),
      hostIds: () => buildings.ids(),
      lastEvents: () => store.events.map((e) => ({ id: e.id, kind: e.kind })),
      probe: (id) => {
        const g = buildings.geometry(id);
        return g ? probe(g, camera, window.innerWidth, window.innerHeight, blockCentres()) : null;
      },
      poseHost: () => poseHost,
    })
  : null;

const connection = new Connection(
  {
    onHello: (h) => {
      store.applyHello(h);
      traffic.clear();
      hud.set({ source: h.server.source === "live" ? `Source: live ${h.server.iface ?? ""}` : "Source: file" });
    },
    onTick: (t) => {
      if (paused) return; // ticks are dropped while paused; resume resyncs
      store.applyTick(t);
      traffic.add(t.ts, t.window_ms, t.stats);
      const r = traffic.rates();
      hud.set({
        traffic: `${formatPps(r.pps)} · ${formatRate(r.bps)}`,
        hosts: formatHosts(buildings.size, t.stats.hosts_total, layout.hiddenHosts),
        drops: `Kernel drops ${t.stats.kernel_drops}`,
        dropsAlert: t.stats.kernel_drops > 0,
      });
    },
    onState: (s) => {
      lastState = s;
      hud.set({ status: describe(s) });
    },
  },
  defaultDeps(),
);
void connection.start();

// ---- Frame loop ----------------------------------------------------------------
const timer = new THREE.Timer();
let rendered = false;
function animate(now?: number): void {
  requestAnimationFrame(animate);
  timer.update(now);
  const dt = Math.min(timer.getDelta(), 0.1);
  buildings.frame(dt, timer.getElapsed());
  lightning.frame(dt);
  particles.frame(dt);
  applyPose();
  stepFlight(dt);
  if (inspector.selected && store.ticks - inspectedAt >= 10) {
    inspectedAt = store.ticks; // refresh about once a second of ticks
    showInspector(inspector.selected);
  }
  controls.update();
  composer.render(dt);
  if (!rendered && store.server) {
    rendered = true;
    debug?.markReady();
  }
}
animate();

window.addEventListener("resize", () => {
  camera.aspect = window.innerWidth / window.innerHeight;
  camera.updateProjectionMatrix();
  renderer.setSize(window.innerWidth, window.innerHeight);
  composer.setSize(window.innerWidth, window.innerHeight);
});
