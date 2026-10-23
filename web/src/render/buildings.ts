// SPDX-License-Identifier: MIT
// Instanced buildings (docs/VISUAL_SPEC.md "Buildings"): one InstancedMesh of
// lit dark-glass boxes, heights from the rate model, fade in/out.
import * as THREE from "three";
import { DEVICE_COLORS, MATERIALS, MAX_BUILDINGS, STATUS } from "../config";
import type { Point } from "../layout/layout";
import type { Host } from "../protocol";
import { CATEGORIES } from "../protocol";
import type { RateState } from "../state/rate";
import { addSample, decayedRate, newRate, stepHeight, targetHeight } from "../state/rate";
import { degradedLevel, sampleEffect } from "../state/status";
import { Outage } from "./effects/outage";
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
  flash: number; // spike flash 1 → 0 over STATUS.flashS
  outage: Outage | null; // host_idle power outage, freed when it completes
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
  private readonly idByIndex: (string | undefined)[] = [];
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
    const existing = this.byId.get(h.id);
    if (existing) {
      // Back before its fade-out or outage finished: keep the building.
      existing.removing = null;
      existing.outage = null;
      this.state.setZ(existing.index, 0);
      this.update(h);
      return;
    }
    const index = this.free.pop() ?? (this.mesh.count < this.mesh.instanceMatrix.count ? this.mesh.count++ : -1);
    if (index < 0) return;
    const b: Building = {
      id: h.id, index, x: at.x, z: at.z,
      base: onPlot ? MATERIALS.plot.height : 0,
      height: 1, rate: newRate(), fadeIn: 0, removing: null, local: h.local, flash: 0, outage: null,
    };
    this.byId.set(h.id, b);
    this.idByIndex[index] = h.id;
    this.update(h);
    this.state.setY(index, 0).setZ(index, 0).setW(index, 0);
  }

  update(h: Host): void {
    const b = this.byId.get(h.id);
    if (!b) return;
    this.tmp.setHex(DEVICE_COLORS[h.device_class] ?? DEVICE_COLORS.unknown ?? 0x00f0ff);
    this.color.setXYZ(b.index, this.tmp.r, this.tmp.g, this.tmp.b);
    this.services.setX(b.index, servicesMask(h));
    this.state.setX(b.index, degradedLevel(h.rtt_ms, h.retrans_rate));
    this.color.needsUpdate = true;
    this.services.needsUpdate = true;
  }

  /** Starts the fade-out; the slot is freed when it completes. */
  remove(id: string): void {
    const b = this.byId.get(id);
    if (b && b.removing === null) b.removing = 0;
  }

  /** Starts the power-outage effect; the slot is freed when it completes. */
  outage(id: string, random?: () => number): void {
    const b = this.byId.get(id);
    if (b && b.removing === null && b.outage === null) b.outage = new Outage(random);
  }

  /** Free instance slots (for tests). */
  get freeSlots(): number {
    return this.free.length;
  }

  /** Host id drawn by an instance (for raycast picking). */
  idAt(index: number): string | null {
    const id = this.idByIndex[index];
    const b = id === undefined ? undefined : this.byId.get(id);
    return b && b.removing === null && b.outage === null ? (id ?? null) : null;
  }

  clear(): void {
    this.idByIndex.length = 0;
    this.byId.clear();
    this.free.length = 0;
    this.mesh.count = 0;
  }

  /**
   * Feeds one tick's per-host bytes into the rate model, starting spike
   * flashes, and returns the ids that should be struck by lightning.
   */
  applyBytes(bytes: Map<string, number>, windowMs: number, tickTs: number): string[] {
    this.nowTs = tickTs;
    const strikes: string[] = [];
    for (const [id, n] of bytes) {
      const b = this.byId.get(id);
      if (!b) continue;
      const prev = decayedRate(b.rate, tickTs);
      const effect = sampleEffect(prev, addSample(b.rate, n, windowMs, tickTs));
      if (effect !== "none") b.flash = 1;
      if (effect === "lightning") strikes.push(id);
    }
    return strikes;
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
      if (b.outage) {
        const o = b.outage.step(dt);
        if (o.done) {
          freed.push(b.id);
          continue;
        }
        scaleY *= o.scaleY;
        this.state.setZ(b.index, o.windowsOff ? -1 : o.dissolve);
      } else if (b.removing !== null) {
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
      b.flash = Math.max(0, b.flash - dt / STATUS.flashS);
      this.state.setY(b.index, b.flash);
      this.state.setW(b.index, b.fadeIn);
    }
    for (const id of freed) {
      const b = this.byId.get(id);
      if (!b) continue;
      this.dummy.scale.set(0, 0, 0);
      this.dummy.updateMatrix();
      this.mesh.setMatrixAt(b.index, this.dummy.matrix);
      this.free.push(b.index);
      this.idByIndex[b.index] = undefined;
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

