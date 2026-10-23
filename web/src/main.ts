import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { parseMessage } from "./protocol";
import "./ui/styles.css";

// Prototype renderer, now fed by the v1 protocol (T0.8b). The shaded city
// (docs/VISUAL_SPEC.md) replaces it in Milestone 3.

const scene = new THREE.Scene();
const fog = new THREE.FogExp2(0x05050a, 0.005);
scene.fog = fog;

const camera = new THREE.PerspectiveCamera(75, window.innerWidth / window.innerHeight, 0.1, 1000);
camera.position.set(60, 40, 60);

const renderer = new THREE.WebGLRenderer({ antialias: true });
renderer.setSize(window.innerWidth, window.innerHeight);
renderer.setClearColor(fog.color);
document.body.appendChild(renderer.domElement);

const controls = new OrbitControls(camera, renderer.domElement);
controls.enableDamping = true;

scene.add(new THREE.GridHelper(300, 100, 0x00f0ff, 0x052535));
scene.add(new THREE.AmbientLight(0x112233));

const maxBuildings = 1000;
const geometry = new THREE.BoxGeometry(2, 1, 2);
geometry.translate(0, 0.5, 0); // origin at the bottom face
const material = new THREE.MeshBasicMaterial({ color: 0x00f0ff, wireframe: true });
const buildingMesh = new THREE.InstancedMesh(geometry, material, maxBuildings);
scene.add(buildingMesh);

interface Host {
  index: number;
  x: number;
  z: number;
  height: number;
  targetHeight: number;
}

const hosts = new Map<string, Host>();
const dummy = new THREE.Object3D();

function getOrCreateHost(ip: string, isLocal: boolean): Host | null {
  const existing = hosts.get(ip);
  if (existing) return existing;
  if (hosts.size >= maxBuildings) return null;

  const angle = Math.random() * Math.PI * 2;
  const radius = isLocal ? 10 + Math.random() * 30 : 100;
  const host: Host = {
    index: hosts.size,
    x: Math.cos(angle) * radius,
    z: Math.sin(angle) * radius,
    height: 1,
    targetHeight: 1,
  };
  hosts.set(ip, host);
  return host;
}

const statusDiv = document.getElementById("status");
const setStatus = (text: string): void => {
  if (statusDiv) statusDiv.textContent = text;
};

// Same origin as the page: the Go binary serves both (docs/DECISIONS.md D3).
const wsUrl = new URL("/ws", window.location.href);
wsUrl.protocol = wsUrl.protocol === "https:" ? "wss:" : "ws:";
const ws = new WebSocket(wsUrl);
ws.onopen = () => setStatus("WS: Connected Live");
ws.onclose = () => setStatus("WS: Disconnected");
ws.onmessage = (event: MessageEvent<string>) => {
  const msg = parseMessage(event.data);
  if (!msg) return;
  for (const h of msg.hosts) getOrCreateHost(h.id, h.local);
  if (msg.type !== "tick") return;
  for (const flow of msg.flows) {
    const grow = flow.bytes / 5000;
    // Flows to multicast/broadcast groups name no host (docs/DECISIONS.md D21).
    for (const id of [flow.src, flow.dst]) {
      const host = hosts.get(id);
      if (host) host.targetHeight = Math.min(60, host.targetHeight + grow);
    }
  }
};

function animate(): void {
  requestAnimationFrame(animate);
  controls.update();

  for (const host of hosts.values()) {
    host.height = THREE.MathUtils.lerp(host.height, host.targetHeight, 0.05);
    dummy.position.set(host.x, 0, host.z);
    dummy.scale.set(1, host.height, 1);
    dummy.updateMatrix();
    buildingMesh.setMatrixAt(host.index, dummy.matrix);
  }
  if (hosts.size > 0) buildingMesh.instanceMatrix.needsUpdate = true;

  renderer.render(scene, camera);
}
animate();

window.addEventListener("resize", () => {
  camera.aspect = window.innerWidth / window.innerHeight;
  camera.updateProjectionMatrix();
  renderer.setSize(window.innerWidth, window.innerHeight);
});
