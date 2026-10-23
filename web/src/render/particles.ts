// Particle freight rendering (docs/VISUAL_SPEC.md "Particle freight"):
// instanced small boxes oriented along their road, additive, category colour.
import * as THREE from "three";
import { CATEGORY_COLORS, PARTICLES } from "../config";
import type { Point } from "../layout/layout";
import type { Road } from "../state/highways";
import { route } from "../state/highways";
import type { Emitter } from "../state/particles";
import { ParticleSystem, along, emissionRate, travelTime } from "../state/particles";
import { glowAttributes, glowMaterial } from "./glow";
import type { EndPoint } from "./highways";
import { ROAD_Y } from "./highways";

export class Particles {
  readonly mesh: THREE.InstancedMesh;
  readonly system: ParticleSystem;
  private emitters: Emitter[] = [];
  private readonly color: THREE.InstancedBufferAttribute;
  private readonly alpha: THREE.InstancedBufferAttribute;
  private readonly dummy = new THREE.Object3D();
  private readonly tmp = new THREE.Color();

  /** Buffers hold the full pool; QualityController lowers system.capacity in low mode. */
  constructor(low: boolean) {
    const capacity = PARTICLES.pool;
    const geo = new THREE.BoxGeometry(...PARTICLES.size);
    ({ color: this.color, alpha: this.alpha } = glowAttributes(geo, capacity));
    this.mesh = new THREE.InstancedMesh(geo, glowMaterial(), capacity);
    this.mesh.count = 0;
    this.mesh.frustumCulled = false;
    this.mesh.name = "particles";
    this.system = new ParticleSystem(low ? PARTICLES.poolLow : capacity);
  }

  /** Rebuilds emitters from the visible roads (call on each tick). */
  setRoads(roads: readonly Road[], lookup: (id: string) => EndPoint | null): void {
    this.emitters = [];
    for (const r of roads) {
      const a = lookup(r.a);
      const b = lookup(r.b);
      if (!a || !b) continue;
      const segs = route(a.point, b.point, a.local && b.local);
      const path: Point[] = [segs[0]?.from ?? a.point, ...segs.map((s) => s.to)];
      const rate = emissionRate(r.pps);
      const duration = travelTime(r.rttMs);
      if (r.forward) this.emitters.push({ key: `${r.key}>`, path, rate, duration, category: r.category });
      if (r.backward) this.emitters.push({ key: `${r.key}<`, path: [...path].reverse(), rate, duration, category: r.category });
    }
  }

  clear(): void {
    this.emitters = [];
    this.system.clear();
  }

  frame(dt: number): void {
    this.system.step(dt, this.emitters);
    const live = this.system.live;
    for (let i = 0; i < live.length; i++) {
      const p = live[i];
      if (!p) continue;
      const { p: at, heading } = along(p.path, p.age / p.duration);
      this.dummy.position.set(at.x, ROAD_Y + 0.3, at.z);
      this.dummy.rotation.set(0, Math.PI / 2 - heading, 0); // long (z) axis along the road
      this.dummy.updateMatrix();
      this.mesh.setMatrixAt(i, this.dummy.matrix);
      this.tmp.setHex(CATEGORY_COLORS[p.category] ?? CATEGORY_COLORS.other ?? 0x8a8fa3);
      this.color.setXYZ(i, this.tmp.r, this.tmp.g, this.tmp.b);
      this.alpha.setX(i, 1);
    }
    this.mesh.count = live.length;
    this.mesh.instanceMatrix.needsUpdate = true;
    this.color.needsUpdate = true;
    this.alpha.needsUpdate = true;
  }
}
