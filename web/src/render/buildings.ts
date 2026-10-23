// Instanced buildings (docs/VISUAL_SPEC.md "Buildings"): one InstancedMesh of
// lit dark-glass boxes, heights from the rate model, fade in/out.
import * as THREE from "three";
import { DEVICE_COLORS, MATERIALS, MAX_BUILDINGS, STATUS } from "../config";
import type { Point } from "../layout/layout";
import type { Host } from "../protocol";
import { CATEGORIES } from "../protocol";
import type { RateState } from "../state/rate";
import { addSample, decayedRate, newRate, stepHeight, targetHeight } from "../state/rate";
import { buildingDepthMaterial, buildingMaterial } from "./materials";

interface Building {
  id: string;
  index: number;
  x: number;
  z: number;
  base: number; // y of the surface it stands on (plot top or ground)
  height: number;
  rate: RateState;
  fadeIn: number; // 0 → 1 over STATUS.fadeInS
  removing: number | null; // seconds into the fade-out, or null
  local: boolean;
}

/** Category bitmask of a host's services, bit order as in CONFIG.md. */
export function servicesMask(h: Host): number {
  let mask = 0;
  for (const s of h.services) {
    const bit = (CATEGORIES as readonly string[]).indexOf(s.category);
    mask |= 1 << (bit >= 0 ? bit : CATEGORIES.length - 1);
  }
  return mask;
}

export class Buildings {
  readonly mesh: THREE.InstancedMesh;
  private readonly byId = new Map<string, Building>();
  private readonly free: number[] = [];
  private readonly color: THREE.InstancedBufferAttribute;
  private readonly services: THREE.InstancedBufferAttribute;
  private readonly state: THREE.InstancedBufferAttribute;
  private readonly dummy = new THREE.Object3D();
  private readonly tmp = new THREE.Color();
  private nowTs = 0;

  constructor(capacity = MAX_BUILDINGS) {
    const geo = new THREE.BoxGeometry(2, 1, 2);
    geo.translate(0, 0.5, 0); // y ∈ [0, 1]
    this.color = new THREE.InstancedBufferAttribute(new Float32Array(capacity * 3), 3);
    this.services = new THREE.InstancedBufferAttribute(new Float32Array(capacity), 1);
    this.state = new THREE.InstancedBufferAttribute(new Float32Array(capacity * 4), 4);
    for (const a of [this.color, this.services, this.state]) a.setUsage(THREE.DynamicDrawUsage);
    geo.setAttribute("aColor", this.color);
    geo.setAttribute("aServices", this.services);
    geo.setAttribute("aState", this.state);
    this.mesh = new THREE.InstancedMesh(geo, buildingMaterial(), capacity);
    this.mesh.customDepthMaterial = buildingDepthMaterial();
    this.mesh.castShadow = true;
    this.mesh.receiveShadow = true;
    this.mesh.count = 0;
    this.mesh.frustumCulled = false; // instances span the whole city
    this.mesh.name = "buildings";
  }

  get size(): number {
    return this.byId.size;
  }

  ids(): string[] {
    return [...this.byId.keys()];
  }

  has(id: string): boolean {
    return this.byId.has(id);
  }

  add(h: Host, at: Point, onPlot: boolean): void {
    if (this.byId.has(h.id)) return;
    const index = this.free.pop() ?? (this.mesh.count < this.mesh.instanceMatrix.count ? this.mesh.count++ : -1);
    if (index < 0) return;
    const b: Building = {
      id: h.id, index, x: at.x, z: at.z,
      base: onPlot ? MATERIALS.plot.height : 0,
      height: 1, rate: newRate(), fadeIn: 0, removing: null, local: h.local,
    };
    this.byId.set(h.id, b);
    this.update(h);
    this.state.setXYZW(index, 0, 0, 0, 0);
  }

  update(h: Host): void {
    const b = this.byId.get(h.id);
    if (!b) return;
    this.tmp.setHex(DEVICE_COLORS[h.device_class] ?? DEVICE_COLORS.unknown ?? 0x00f0ff);
    this.color.setXYZ(b.index, this.tmp.r, this.tmp.g, this.tmp.b);
    this.services.setX(b.index, servicesMask(h));
    this.color.needsUpdate = true;
    this.services.needsUpdate = true;
  }

  /** Starts the fade-out; the slot is freed when it completes. */
  remove(id: string): void {
    const b = this.byId.get(id);
    if (b && b.removing === null) b.removing = 0;
  }

  clear(): void {
    this.byId.clear();
    this.free.length = 0;
    this.mesh.count = 0;
  }

  /** Feeds one tick's per-host bytes into the rate model. */
  applyBytes(bytes: Map<string, number>, windowMs: number, tickTs: number): void {
    this.nowTs = tickTs;
    for (const [id, n] of bytes) {
      const b = this.byId.get(id);
      if (b) addSample(b.rate, n, windowMs, tickTs);
    }
  }

  /** Advances animation by dt seconds and writes instance data. */
  frame(dt: number, timeS = 0): string[] {
    const shader = (this.mesh.material as THREE.Material).userData.shader as
      | { uniforms: { uCCTime: { value: number } } }
      | undefined;
    if (shader) shader.uniforms.uCCTime.value = timeS;
    const freed: string[] = [];
    for (const b of this.byId.values()) {
      const target = targetHeight(decayedRate(b.rate, this.nowTs));
      b.height = stepHeight(b.height, target, dt);
      b.fadeIn = Math.min(1, b.fadeIn + dt / STATUS.fadeInS);
      let scaleY = b.height * b.fadeIn;
      if (b.removing !== null) {
        b.removing += dt;
        scaleY *= Math.max(0, 1 - b.removing / STATUS.fadeOutS);
        if (b.removing >= STATUS.fadeOutS) {
          freed.push(b.id);
          continue;
        }
      }
      this.dummy.position.set(b.x, b.base, b.z);
      this.dummy.scale.set(1, Math.max(scaleY, 1e-3), 1);
      this.dummy.updateMatrix();
      this.mesh.setMatrixAt(b.index, this.dummy.matrix);
      this.state.setW(b.index, b.fadeIn);
    }
    for (const id of freed) {
      const b = this.byId.get(id);
      if (!b) continue;
      this.dummy.scale.set(0, 0, 0);
      this.dummy.updateMatrix();
      this.mesh.setMatrixAt(b.index, this.dummy.matrix);
      this.free.push(b.index);
      this.byId.delete(id);
    }
    this.mesh.instanceMatrix.needsUpdate = true;
    this.state.needsUpdate = true;
    return freed;
  }

  /** Building footprint centre, base and displayed height (for probes and camera poses). */
  geometry(id: string): { x: number; z: number; base: number; height: number; local: boolean } | null {
    const b = this.byId.get(id);
    return b ? { x: b.x, z: b.z, base: b.base, height: b.height * b.fadeIn, local: b.local } : null;
  }

  /** Current packet-time rate of a building (bytes/s). */
  rateOf(id: string): number {
    const b = this.byId.get(id);
    return b ? decayedRate(b.rate, this.nowTs) : 0;
  }
}

