import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { CAMERA, MAX_BUILDINGS, PALETTE, SCENE } from "./config";
import { debugEnabled, installDebug } from "./debug";
import type { ConnectionState } from "./net/connection";
import { Connection, defaultDeps } from "./net/connection";
import { INVALID_LINK_MESSAGE } from "./net/session";
import { rendererInfo } from "./render/rendererInfo";
import { Store } from "./state/store";
import "./ui/styles.css";

// App wiring (T3.1). The buildings are still the prototype's wireframe boxes;
// the shaded city replaces them from T3.3a/T3.4 (docs/VISUAL_SPEC.md).

const params = new URLSearchParams(window.location.search);

const renderer = new THREE.WebGLRenderer({ antialias: true, powerPreference: "high-performance" });
const gpu = rendererInfo(renderer.getContext());
const forced = params.get("quality");
const quality: "high" | "low" = forced === "high" || forced === "low" ? forced : gpu.software ? "low" : "high";
renderer.setPixelRatio(quality === "low" ? 1 : Math.min(window.devicePixelRatio, SCENE.maxPixelRatio));
renderer.setSize(window.innerWidth, window.innerHeight);
renderer.setClearColor(PALETTE.bg);
document.body.appendChild(renderer.domElement);

const scene = new THREE.Scene();
scene.fog = new THREE.FogExp2(PALETTE.bg, SCENE.fogDensity);
const camera = new THREE.PerspectiveCamera(CAMERA.fov, window.innerWidth / window.innerHeight, CAMERA.near, CAMERA.far);
camera.position.set(...CAMERA.position);
const controls = new OrbitControls(camera, renderer.domElement);
controls.enableDamping = true;
controls.dampingFactor = CAMERA.damping;
scene.add(new THREE.GridHelper(SCENE.groundSize, SCENE.gridDivisions, PALETTE.gridMajor, PALETTE.gridMinor));

// ---- Prototype buildings (replaced in T3.4) ------------------------------
const geometry = new THREE.BoxGeometry(2, 1, 2);
geometry.translate(0, 0.5, 0);
const buildingMesh = new THREE.InstancedMesh(
  geometry,
  new THREE.MeshBasicMaterial({ color: PALETTE.gridMajor, wireframe: true }),
  MAX_BUILDINGS,
);
buildingMesh.count = 0;
scene.add(buildingMesh);

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
  },
  hostAdded(h) {
    if (buildings.size >= MAX_BUILDINGS) return;
    const index = free.pop() ?? buildingMesh.count++;
    const angle = Math.random() * Math.PI * 2;
    const radius = h.local ? 10 + Math.random() * 30 : 100;
    buildings.set(h.id, { index, x: Math.cos(angle) * radius, z: Math.sin(angle) * radius, height: 1, target: 1 });
  },
  hostRemoved(id) {
    const b = buildings.get(id);
    if (!b) return;
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
        lit: false,
        wireframe: true,
        shadows: false,
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
    dummy.position.set(b.x, 0, b.z);
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
