import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { parseBatch } from "./prototype";
import "./ui/styles.css";

// Prototype renderer carried over unchanged in behaviour by T0.2. The shaded city
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
  for (const flow of parseBatch(event.data)) {
    const grow = flow.byte_count / 5000;
    const src = getOrCreateHost(flow.src_ip, flow.src_is_local);
    const dst = getOrCreateHost(flow.dst_ip, flow.dst_is_local);
    if (src) src.targetHeight = Math.min(60, src.targetHeight + grow);
    if (dst) dst.targetHeight = Math.min(60, dst.targetHeight + grow);
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
