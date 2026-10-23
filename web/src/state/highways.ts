// Highway model (docs/VISUAL_SPEC.md "Highways"): one road per host pair with
// recent traffic, width from the pair's rate, colour from its dominant
// category, fading out after the last packet. Pure: no three.js.
import { HIGHWAYS } from "../config";
import type { Point } from "../layout/layout";
import type { Flow } from "../protocol";
import { asCategory } from "../protocol";

const CATEGORY_HALF_LIFE_S = 3; // "dominant category over the last 3 s"

interface Pair {
  a: string;
  b: string;
  rate: number; // bytes/s EWMA (half-life 2 s, like buildings)
  lastSeen: number; // packet time ms
  lastSample: number;
  categories: Map<string, number>; // decayed bytes per category
}

export interface Road {
  key: string;
  a: string;
  b: string;
  width: number;
  category: string;
  opacity: number;
  rate: number;
}

export interface Segment {
  from: Point;
  to: Point;
}

export const pairKey = (a: string, b: string): string => (a < b ? `${a}|${b}` : `${b}|${a}`);

/** Road width: clamp(0.2 + 0.6·log10(1 + rate/1000), 0.2, 3). */
export function roadWidth(rateBps: number): number {
  const w = HIGHWAYS.minWidth + HIGHWAYS.widthScale * Math.log10(1 + Math.max(0, rateBps) / 1000);
  return Math.min(HIGHWAYS.maxWidth, Math.max(HIGHWAYS.minWidth, w));
}

/** Opacity fades from 0.35 to 0 over 3 s after the last packet. */
export function roadOpacity(sinceLastMs: number): number {
  return HIGHWAYS.opacity * Math.max(0, 1 - Math.max(0, sinceLastMs) / (HIGHWAYS.fadeS * 1000));
}

/** L-shaped route (x first, then z) between local buildings; straight otherwise. */
export function route(from: Point, to: Point, bothLocal: boolean): Segment[] {
  if (!bothLocal || from.x === to.x || from.z === to.z) return [{ from, to }];
  const corner = { x: to.x, z: from.z };
  return [
    { from, to: corner },
    { from: corner, to },
  ];
}

export class HighwayModel {
  private readonly pairs = new Map<string, Pair>();
  nowTs = 0;

  /** Adds a tick's flows between known hosts (isHost filters out group addresses). */
  applyTick(flows: readonly Flow[], windowMs: number, ts: number, isHost: (id: string) => boolean): void {
    this.nowTs = ts;
    const bytes = new Map<string, { a: string; b: string; total: number; cats: Map<string, number> }>();
    for (const f of flows) {
      if (f.src === f.dst || !isHost(f.src) || !isHost(f.dst)) continue;
      const key = pairKey(f.src, f.dst);
      let e = bytes.get(key);
      if (!e) {
        e = { a: f.src < f.dst ? f.src : f.dst, b: f.src < f.dst ? f.dst : f.src, total: 0, cats: new Map() };
        bytes.set(key, e);
      }
      e.total += f.bytes;
      const cat = asCategory(f.category);
      e.cats.set(cat, (e.cats.get(cat) ?? 0) + f.bytes);
    }
    for (const [key, e] of bytes) {
      let p = this.pairs.get(key);
      if (!p) {
        p = { a: e.a, b: e.b, rate: 0, lastSeen: ts, lastSample: 0, categories: new Map() };
        this.pairs.set(key, p);
      }
      const sample = e.total / (windowMs / 1000);
      const dt = p.lastSample === 0 ? Infinity : (ts - p.lastSample) / 1000;
      p.rate += (1 - 2 ** (-dt / 2)) * (sample - p.rate);
      const keep = 2 ** (-Math.max(0, (ts - p.lastSample) / 1000) / CATEGORY_HALF_LIFE_S);
      for (const [c, v] of p.categories) p.categories.set(c, v * keep);
      for (const [c, v] of e.cats) p.categories.set(c, (p.categories.get(c) ?? 0) + v);
      p.lastSample = ts;
      p.lastSeen = ts;
    }
  }

  /** Forgets every road touching a removed host. */
  removeHost(id: string): void {
    for (const [k, p] of this.pairs) if (p.a === id || p.b === id) this.pairs.delete(k);
  }

  clear(): void {
    this.pairs.clear();
  }

  /** Visible roads at packet time nowTs: the HIGHWAYS.max busiest, faded ones dropped. */
  roads(nowTs = this.nowTs): Road[] {
    const out: Road[] = [];
    for (const [key, p] of this.pairs) {
      const opacity = roadOpacity(nowTs - p.lastSeen);
      if (opacity <= 0) {
        this.pairs.delete(key); // fully faded: free it
        continue;
      }
      let category = "other";
      let best = -1;
      for (const [c, v] of p.categories) {
        if (v > best || (v === best && c < category)) [category, best] = [c, v];
      }
      const rate = p.rate * 2 ** (-Math.max(0, nowTs - p.lastSample) / 1000 / 2);
      out.push({ key, a: p.a, b: p.b, width: roadWidth(rate), category, opacity, rate });
    }
    out.sort((x, y) => y.rate - x.rate || (x.key < y.key ? -1 : 1));
    return out.slice(0, HIGHWAYS.max);
  }
}
