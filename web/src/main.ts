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
import { poseFor, probe } from "./render/probe";
import { rendererInfo } from "./render/rendererInfo";
import { Plots, addLights, configureRenderer, createGround, setEnvironment } from "./render/scene";
import { bytesByHost } from "./state/rate";
import { Store } from "./state/store";
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

addLights(scene, quality);
const pmrem = new THREE.PMREMGenerator(renderer);
setEnvironment(scene, pmrem.fromScene(new RoomEnvironment(), LIGHTING.environment.sigma).texture);
pmrem.dispose();
scene.add(createGround());
const plots = new Plots();
scene.add(plots.mesh);
const buildings = new Buildings();
scene.add(buildings.mesh);
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
    layout.reset();
    plots.update([]);
    firstTickTs = 0;
  },
  hostAdded: place,
  hostUpdated: (h) => buildings.update(h),
  hostRemoved(id) {
    buildings.remove(id);
  },
  tick(t) {
    if (firstTickTs === 0) firstTickTs = t.ts;
    lastTickTs = t.ts;
    buildings.applyBytes(bytesByHost(t.flows), t.window_ms, t.ts);
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
const statusDiv = document.getElementById("status");
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

function describe(s: ConnectionState): string {
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
        highways: 0,
        particles: 0,
        ticks: store.ticks,
        events: store.events.length,
        renderer: gpu.name,
        software: gpu.software,
        quality,
        lit: buildings.mesh.material instanceof THREE.MeshStandardMaterial,
        wireframe: (buildings.mesh.material as THREE.MeshStandardMaterial).wireframe,
        shadows: renderer.shadowMap.enabled && buildings.mesh.castShadow,
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
    onHello: (h) => store.applyHello(h),
    onTick: (t) => store.applyTick(t),
    onState: (s) => setText(statusDiv, describe(s)),
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
  applyPose();
  controls.update();
  renderer.render(scene, camera);
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
});
