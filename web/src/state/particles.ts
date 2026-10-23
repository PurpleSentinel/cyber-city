// Particle freight model (docs/VISUAL_SPEC.md "Particle freight"). Pure: the
// renderer reads positions from here.
import { PARTICLES } from "../config";
import type { Point } from "../layout/layout";

/** Particles per second for a road: clamp(1 + log2(1 + pps), 1, 20). */
export function emissionRate(pps: number): number {
  return Math.min(PARTICLES.maxRate, Math.max(1, 1 + Math.log2(1 + Math.max(0, pps))));
}

/** Seconds to travel a road: clamp(0.4 + rtt·0.02, 0.4, 4.0), 1.5 if unknown. */
export function travelTime(rttMs: number | null): number {
  if (rttMs === null) return PARTICLES.travelUnknownS;
  return Math.min(PARTICLES.travelMaxS, Math.max(PARTICLES.travelBaseS, PARTICLES.travelBaseS + rttMs * PARTICLES.travelPerRttMs));
}

/** Position at fraction t (0..1) along a polyline, and the segment heading. */
export function along(path: readonly Point[], t: number): { p: Point; heading: number } {
  const first = path[0] ?? { x: 0, z: 0 };
  let total = 0;
  for (let i = 1; i < path.length; i++) total += dist(path[i - 1] as Point, path[i] as Point);
  let remaining = Math.min(1, Math.max(0, t)) * total;
  for (let i = 1; i < path.length; i++) {
    const a = path[i - 1] as Point;
    const b = path[i] as Point;
    const d = dist(a, b);
    if (remaining <= d || i === path.length - 1) {
      const f = d === 0 ? 0 : Math.min(1, remaining / d);
      return { p: { x: a.x + (b.x - a.x) * f, z: a.z + (b.z - a.z) * f }, heading: Math.atan2(b.z - a.z, b.x - a.x) };
    }
    remaining -= d;
  }
  return { p: first, heading: 0 };
}

const dist = (a: Point, b: Point) => Math.hypot(b.x - a.x, b.z - a.z);

export interface Particle {
  path: Point[];
  age: number;
  duration: number;
  category: string;
}

export interface Emitter {
  key: string; // road key + direction
  path: Point[];
  rate: number; // particles/s
  duration: number;
  category: string;
}

/** Fixed-size particle pool with per-emitter fractional accumulators. */
export class ParticleSystem {
  readonly live: Particle[] = [];
  private readonly acc = new Map<string, number>();

  constructor(public capacity: number) {}

  /** Spawns due particles for the active emitters and ages the rest. */
  step(dt: number, emitters: readonly Emitter[]): void {
    for (let i = this.live.length - 1; i >= 0; i--) {
      const p = this.live[i] as Particle;
      p.age += dt;
      if (p.age >= p.duration) this.live.splice(i, 1);
    }
    const active = new Set<string>();
    for (const e of emitters) {
      active.add(e.key);
      let acc = (this.acc.get(e.key) ?? 0) + e.rate * dt;
      while (acc >= 1) {
        acc -= 1;
        if (this.live.length >= this.capacity) continue; // pool full: skip
        this.live.push({ path: e.path, age: 0, duration: e.duration, category: e.category });
      }
      this.acc.set(e.key, acc);
    }
    for (const k of this.acc.keys()) if (!active.has(k)) this.acc.delete(k);
  }

  clear(): void {
    this.live.length = 0;
    this.acc.clear();
  }
}
