// SPDX-License-Identifier: MIT
// Meteor shower for flood events (docs/VISUAL_SPEC.md "Event effects"): the
// sky turns alert red, particles fall onto the target, and a wireframe shield
// flickers around it.
import * as THREE from "three";
import { EFFECTS, PALETTE } from "../../config";
import type { Footprint } from "./searchlight";

const M = EFFECTS.meteor;

/** How far the sky is lerped towards alertSky t seconds into a shower (0..1). */
export function skyMix(t: number): number {
  if (t < 0 || t >= M.durationS) return 0;
  if (t < M.rampInS) return t / M.rampInS;
  const out = M.durationS - M.rampOutS;
  return t <= out ? 1 : 1 - (t - out) / M.rampOutS;
}

interface Shower {
  dst: string;
  age: number;
  sinceFlicker: number;
  points: THREE.Points;
  start: Float32Array; // per particle start x, y, z
  speed: Float32Array;
  travelled: Float32Array;
  shield: THREE.Mesh;
}

export class Meteors {
  readonly group = new THREE.Group();
  private readonly showers: Shower[] = [];
  private readonly pointMaterial = new THREE.PointsMaterial({
    color: PALETTE.alert,
    size: 1.5,
    transparent: true,
    blending: THREE.AdditiveBlending,
    depthWrite: false,
    toneMapped: false,
  });
  private readonly sky = new THREE.Color();
  private readonly bg = new THREE.Color(PALETTE.bg);
  private readonly alertSky = new THREE.Color(PALETTE.alertSky);

  constructor(
    private readonly footprint: (id: string) => Footprint | null,
    private readonly random: () => number = Math.random,
  ) {
    this.group.name = "meteors";
  }

  get active(): number {
    return this.showers.length;
  }

  /** Starts a shower on dst; false when the cap is reached or dst is not drawn. */
  start(dst: string, low = false): boolean {
    const f = this.footprint(dst);
    if (this.showers.length >= EFFECTS.maxMeteors || !f) return false;
    const n = low ? M.particlesLow : M.particles;
    const geo = new THREE.BufferGeometry();
    geo.setAttribute("position", new THREE.BufferAttribute(new Float32Array(n * 3), 3));
    const points = new THREE.Points(geo, this.pointMaterial);
    points.frustumCulled = false;
    const shieldMaterial = new THREE.MeshBasicMaterial({
      color: PALETTE.alert,
      wireframe: true,
      transparent: true,
      opacity: 0.6,
      depthWrite: false,
      toneMapped: false,
    });
    const shield = new THREE.Mesh(new THREE.IcosahedronGeometry(Math.max(6, f.height * 0.6), 1), shieldMaterial);
    const s: Shower = {
      dst, age: 0, sinceFlicker: 0, points, shield,
      start: new Float32Array(n * 3), speed: new Float32Array(n), travelled: new Float32Array(n),
    };
    for (let i = 0; i < n; i++) this.spawn(s, i, f, this.random() * M.startY);
    this.group.add(points, shield);
    this.showers.push(s);
    this.place(s, f);
    return true;
  }

  /** Sky colour for this frame: the strongest running shower wins. */
  skyColor(): THREE.Color {
    let k = 0;
    for (const s of this.showers) k = Math.max(k, skyMix(s.age));
    return this.sky.copy(this.bg).lerp(this.alertSky, k);
  }

  frame(dt: number): void {
    for (let i = this.showers.length - 1; i >= 0; i--) {
      const s = this.showers[i] as Shower;
      s.age += dt;
      const f = this.footprint(s.dst);
      if (s.age >= M.durationS || !f) {
        this.group.remove(s.points, s.shield);
        s.points.geometry.dispose();
        s.shield.geometry.dispose();
        (s.shield.material as THREE.Material).dispose();
        this.showers.splice(i, 1);
        continue;
      }
      s.sinceFlicker += dt;
      if (s.sinceFlicker >= 1 / M.flickerHz) {
        s.sinceFlicker = 0;
        (s.shield.material as THREE.MeshBasicMaterial).opacity = 0.1 + this.random() * 0.5;
      }
      for (let p = 0; p < s.speed.length; p++) s.travelled[p] = (s.travelled[p] ?? 0) + (s.speed[p] ?? 0) * dt;
      this.place(s, f);
    }
  }

  /** Particle p of s starts again from the top, already `ahead` units along. */
  private spawn(s: Shower, p: number, f: Footprint, ahead = 0): void {
    const a = this.random() * Math.PI * 2;
    const r = Math.sqrt(this.random()) * M.radius;
    s.start[p * 3] = f.x + Math.cos(a) * r;
    s.start[p * 3 + 1] = M.startY;
    s.start[p * 3 + 2] = f.z + Math.sin(a) * r;
    s.speed[p] = M.speedMin + this.random() * (M.speedMax - M.speedMin);
    s.travelled[p] = ahead;
  }

  private place(s: Shower, f: Footprint): void {
    const roof = f.base + f.height;
    s.shield.position.set(f.x, roof * 0.5, f.z);
    const pos = s.points.geometry.getAttribute("position") as THREE.BufferAttribute;
    for (let p = 0; p < s.speed.length; p++) {
      let sx = s.start[p * 3] ?? 0;
      let sy = s.start[p * 3 + 1] ?? 0;
      let sz = s.start[p * 3 + 2] ?? 0;
      let len = Math.hypot(f.x - sx, roof - sy, f.z - sz);
      if ((s.travelled[p] ?? 0) >= len) {
        this.spawn(s, p, f); // landed: fall again from the sky
        sx = s.start[p * 3] ?? 0;
        sy = s.start[p * 3 + 1] ?? 0;
        sz = s.start[p * 3 + 2] ?? 0;
        len = Math.hypot(f.x - sx, roof - sy, f.z - sz);
      }
      const k = len > 0 ? (s.travelled[p] ?? 0) / len : 1;
      pos.setXYZ(p, sx + (f.x - sx) * k, sy + (roof - sy) * k, sz + (f.z - sz) * k);
    }
    pos.needsUpdate = true;
  }
}
