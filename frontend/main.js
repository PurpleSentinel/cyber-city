import * as THREE from 'three';
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js';

const scene = new THREE.Scene();
scene.fog = new THREE.FogExp2(0x05050a, 0.005);

const camera = new THREE.PerspectiveCamera(75, window.innerWidth / window.innerHeight, 0.1, 1000);
camera.position.set(60, 40, 60);

const renderer = new THREE.WebGLRenderer({ antialias: true });
renderer.setSize(window.innerWidth, window.innerHeight);
renderer.setClearColor(scene.fog.color);
document.body.appendChild(renderer.domElement);

const controls = new OrbitControls(camera, renderer.domElement);
controls.enableDamping = true;

// Grid Environment
const grid = new THREE.GridHelper(300, 100, 0x00f0ff, 0x052535);
scene.add(grid);

const ambientLight = new THREE.AmbientLight(0x112233);
scene.add(ambientLight);

// Instanced Building Setup
const maxBuildings = 1000;
const geometry = new THREE.BoxGeometry(2, 1, 2);
geometry.translate(0, 0.5, 0); // Origin at bottom face
const material = new THREE.MeshBasicMaterial({ color: 0x00f0ff, wireframe: true });
const buildingMesh = new THREE.InstancedMesh(geometry, material, maxBuildings);
scene.add(buildingMesh);

const hostMap = new Map(); // IP -> { index, height, targetHeight }
let buildingCount = 0;

function getOrCreateHost(ip, isLocal) {
    if (hostMap.has(ip)) return hostMap.get(ip);
    if (buildingCount >= maxBuildings) return null;

    const index = buildingCount++;
    const dummy = new THREE.Object3D();
    
    // Position allocation based on layout matrix rules
    let x, z;
    if (isLocal) {
        const angle = Math.random() * Math.PI * 2;
        const radius = 10 + Math.random() * 30;
        x = Math.cos(angle) * radius;
        z = Math.sin(angle) * radius;
    } else {
        const angle = Math.random() * Math.PI * 2;
        x = Math.cos(angle) * 100;
        z = Math.sin(angle) * 100;
    }
    
    dummy.position.set(x, 0, z);
    dummy.updateMatrix();
    buildingMesh.setMatrixAt(index, dummy.matrix);
    
    const hostData = { index, x, z, height: 1, targetHeight: 1 };
    hostMap.set(ip, hostData);
    return hostData;
}

// WebSocket Management
const ws = new WebSocket('ws://localhost:8080/ws');
const statusDiv = document.getElementById('status');

ws.onopen = () => statusDiv.innerText = 'WS: Connected Live';
ws.onclose = () => statusDiv.innerText = 'WS: Disconnected';
ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    data.forEach(pkg => {
        const srcHost = getOrCreateHost(pkg.src_ip, pkg.src_is_local);
        const dstHost = getOrCreateHost(pkg.dst_ip, pkg.dst_is_local);
        
        if (srcHost) srcHost.targetHeight = Math.min(60, srcHost.targetHeight + (pkg.byte_count / 5000));
        if (dstHost) dstHost.targetHeight = Math.min(60, dstHost.targetHeight + (pkg.byte_count / 5000));
    });
};

// Frame Animation Loop
function animate() {
    requestAnimationFrame(animate);
    controls.update();

    const dummy = new THREE.Object3D();
    hostMap.forEach((host) => {
        // Smooth scaling factor
        host.height = THREE.MathUtils.lerp(host.height, host.targetHeight, 0.05);
        
        dummy.position.set(host.x, 0, host.z);
        dummy.scale.set(1, host.height, 1);
        dummy.updateMatrix();
        buildingMesh.setMatrixAt(host.index, dummy.matrix);
    });
    
    if (hostMap.size > 0) {
        buildingMesh.instanceMatrix.needsUpdate = true;
    }

    renderer.render(scene, camera);
}
animate();

window.addEventListener('resize', () => {
    camera.aspect = window.innerWidth / window.innerHeight;
    camera.updateProjectionMatrix();
    renderer.setSize(window.innerWidth, window.innerHeight);
});
