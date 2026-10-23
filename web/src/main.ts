import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { RoomEnvironment } from "three/examples/jsm/environments/RoomEnvironment.js";
import { CAMERA, DEVICE_COLORS, LIGHTING, MAX_BUILDINGS, MATERIALS, PALETTE, SCENE } from "./config";
import { Layout } from "./layout/layout";
import { debugEnabled, installDebug } from "./debug";
import type { ConnectionState } from "./net/connection";
import { Connection, defaultDeps } from "./net/connection";
import { INVALID_LINK_MESSAGE } from "./net/session";
import { rendererInfo } from "./render/rendererInfo";
import { Plots, addLights, configureRenderer, createGround, setEnvironment } from "./render/scene";
import { Store } from "./state/store";
import "./ui/styles.css";

// App wiring. Lighting, ground and plots follow docs/VISUAL_SPEC.md (T3.3a);
// buildings are simple lit boxes until the dark-glass material lands in T3.4.

const params = new URLSearchParams(window.location.search);

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
const layout = new Layout();

// ---- Interim buildings (lit boxes; replaced by the T3.4 material) ---------
const geometry = new THREE.BoxGeometry(2, 1, 2);
geometry.translate(0, 0.5, 0);
const buildingMesh = new THREE.InstancedMesh(
  geometry,
  new THREE.MeshStandardMaterial({ roughness: MATERIALS.building.roughness, metalness: MATERIALS.building.metalness }),
  MAX_BUILDINGS,
);
buildingMesh.castShadow = true;
buildingMesh.receiveShadow = true;
buildingMesh.count = 0;
scene.add(buildingMesh);
const tint = new THREE.Color();

interface Building {
  index: number;
  x: number;
  z: number;
  height: number;
  target: number;
}
const buildings = new Map<string, Building>();
const free: number[] = [];
const dummy = new THREE.Object3D();

const store = new Store();
store.subscribe({
  reset() {
    buildings.clear();
    free.length = 0;
    buildingMesh.count = 0;
    layout.reset();
    plots.update([]);
  },
  hostAdded(h) {
    if (buildings.size >= MAX_BUILDINGS) return;
    const kind = store.districts.get(h.district)?.kind ?? (h.local ? "lan" : "outlands");
    const p = layout.place(h.id, h.district, kind, h.geo?.sector ?? null);
    if (!p) return;
    plots.update(layout.blocks());
    const index = free.pop() ?? buildingMesh.count++;
    buildingMesh.setColorAt(index, tint.set(DEVICE_COLORS[h.device_class] ?? PALETTE.hud).multiplyScalar(0.35));
    if (buildingMesh.instanceColor) buildingMesh.instanceColor.needsUpdate = true;
    buildings.set(h.id, { index, x: p.x, z: p.z, height: 1, target: 1 });
  },
  hostRemoved(id) {
    const b = buildings.get(id);
    if (!b) return;
    layout.release(id);
    buildings.delete(id);
    dummy.scale.set(0, 0, 0);
    dummy.updateMatrix();
    buildingMesh.setMatrixAt(b.index, dummy.matrix);
    free.push(b.index);
  },
  tick(t) {
    for (const flow of t.flows) {
      // Flows to multicast/broadcast groups name no host (docs/DECISIONS.md D21).
      for (const id of [flow.src, flow.dst]) {
        const b = buildings.get(id);
        if (b) b.target = Math.min(60, b.target + flow.bytes / 5000);
      }
    }
  },
});

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
        lit: true,
        wireframe: false,
        shadows: renderer.shadowMap.enabled,
      }),
      hostIds: () => [...buildings.keys()],
      lastEvents: () => store.events.map((e) => ({ id: e.id, kind: e.kind })),
      probe: () => null,
      poseHost: () => null,
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
let firstFrame = true;
function animate(): void {
  requestAnimationFrame(animate);
  controls.update();
  for (const b of buildings.values()) {
    b.height = THREE.MathUtils.lerp(b.height, b.target, 0.05);
    dummy.position.set(b.x, MATERIALS.plot.height, b.z);
    dummy.scale.set(1, b.height, 1);
    dummy.updateMatrix();
    buildingMesh.setMatrixAt(b.index, dummy.matrix);
  }
  buildingMesh.instanceMatrix.needsUpdate = true;
  renderer.render(scene, camera);
  if (firstFrame && store.server) {
    firstFrame = false;
    debug?.markReady();
  }
}
animate();

window.addEventListener("resize", () => {
  camera.aspect = window.innerWidth / window.innerHeight;
  camera.updateProjectionMatrix();
  renderer.setSize(window.innerWidth, window.innerHeight);
});
