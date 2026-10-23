// SPDX-License-Identifier: MIT
// Deterministic city layout (docs/VISUAL_SPEC.md "Layout"). No three.js here:
// pure geometry, tested by Vitest.
//
// Positions depend only on ids except where probing resolves a collision: a
// host whose home slot is free always lands there, and a placed building never
// moves (DECISIONS D24).
import { LAYOUT } from "../config";
import { fnv1a32 } from "./hash";

export interface Point {
  x: number;
  z: number;
}

export type DistrictKind = "lan" | "server" | "outlands";

const BLOCKS = LAYOUT.gridBlocks * LAYOUT.gridBlocks; // 9
const SLOTS_PER_BLOCK = LAYOUT.slotsPerSide * LAYOUT.slotsPerSide; // 256
const BLOCK_STRIDE = LAYOUT.blockSize + LAYOUT.avenue; // 80

/** Centre of a local district block (index 0..8, row-major, 4 = centre). */
export function blockCentre(index: number): Point {
  const col = index % LAYOUT.gridBlocks;
  const row = Math.floor(index / LAYOUT.gridBlocks);
  return { x: (col - 1) * BLOCK_STRIDE, z: (row - 1) * BLOCK_STRIDE };
}

/** Centre of slot s (0..255) inside the block centred at c. */
export function slotCentre(c: Point, s: number): Point {
  const half = LAYOUT.blockSize / 2;
  const col = s % LAYOUT.slotsPerSide;
  const row = Math.floor(s / LAYOUT.slotsPerSide);
  return {
    x: c.x - half + col * LAYOUT.slotPitch + LAYOUT.slotPitch / 2,
    z: c.z - half + row * LAYOUT.slotPitch + LAYOUT.slotPitch / 2,
  };
}

interface Ring {
  radius: number;
  perSector: number;
}

/** Outlands rings: radius r = 180 + 8i; floor(2πr/4) slots split over 8 sectors. */
export const RINGS: readonly Ring[] = Array.from({ length: LAYOUT.outlands.rings }, (_, i) => {
  const radius = LAYOUT.outlands.innerRadius + i * LAYOUT.outlands.ringStep;
  const slots = Math.floor((2 * Math.PI * radius) / LAYOUT.outlands.arcSlot);
  return { radius, perSector: Math.floor(slots / LAYOUT.sectors.length) };
});

const SECTOR_SLOTS = RINGS.reduce((n, r) => n + r.perSector, 0);

/** Position of the k-th slot of a sector (counted ring by ring, inner first). */
export function outlandsSlot(sector: number, k: number): Point {
  let ring = RINGS[RINGS.length - 1] as Ring;
  for (const r of RINGS) {
    if (k < r.perSector) {
      ring = r;
      break;
    }
    k -= r.perSector;
  }
  const width = 360 / LAYOUT.sectors.length;
  const deg = sector * width - width / 2 + ((k + 0.5) * width) / ring.perSector;
  const rad = (deg * Math.PI) / 180;
  // N is −z; angles increase clockwise seen from above (+x is east).
  return { x: ring.radius * Math.sin(rad), z: -ring.radius * Math.cos(rad) };
}

/** Sector index for an outlands host: geo sector, else a hash of the address prefix. */
export function sectorFor(id: string, geoSector: string | null): number {
  if (geoSector) {
    const i = (LAYOUT.sectors as readonly string[]).indexOf(geoSector);
    if (i >= 0) return i;
  }
  return fnv1a32(prefixKey(id)) % LAYOUT.sectors.length;
}

/** First two IPv4 octets or first 32 bits of IPv6; the id itself when anonymised. */
function prefixKey(id: string): string {
  if (id.startsWith("anon-")) return id;
  if (id.includes(":")) return id.split(":").slice(0, 2).join(":");
  return id.split(".").slice(0, 2).join(".");
}

export interface Placement {
  point: Point;
  district: string;
}

/** Assigns districts to blocks and hosts to slots; remembers every placement. */
export class Layout {
  private readonly blockOf = new Map<string, number | null>(); // district → block (null: hidden)
  private readonly blockTaken: (string | null)[] = new Array<string | null>(BLOCKS).fill(null);
  private readonly slots = new Map<string, (string | null)[]>(); // occupancy per block/sector key
  private readonly placed = new Map<string, { key: string; slot: number; point: Point }>();
  hiddenDistricts = 0;
  hiddenHosts = 0;

  /** Block index for a local district, assigning one on first use. */
  blockFor(districtId: string, kind: DistrictKind): number | null {
    const known = this.blockOf.get(districtId);
    if (known !== undefined) return known;
    const order =
      kind === "server"
        ? LAYOUT.serverBlockOrder
        : Array.from({ length: BLOCKS }, (_, i) => (fnv1a32(districtId) + i) % BLOCKS);
    const block = order.find((b) => this.blockTaken[b] === null) ?? null;
    if (block === null) this.hiddenDistricts++;
    else this.blockTaken[block] = districtId;
    this.blockOf.set(districtId, block);
    return block;
  }

  /** Where host id stands, or null if it cannot be drawn (hidden district or full). */
  place(id: string, districtId: string, kind: DistrictKind, geoSector: string | null = null): Point | null {
    const prev = this.placed.get(id);
    if (prev) return prev.point;
    if (kind === "outlands") {
      // A full sector spills into its neighbours (s, s+1, s−1, s+2, …) rather
      // than hiding buildings, e.g. when GeoIP puts most hosts in one country.
      const home = sectorFor(id, geoSector);
      const n = LAYOUT.sectors.length;
      for (let i = 0; i < n; i++) {
        const step = Math.ceil(i / 2) * (i % 2 === 1 ? 1 : -1);
        const sector = (((home + step) % n) + n) % n;
        const p = this.claim(id, `sector:${sector}`, SECTOR_SLOTS, (slot) => outlandsSlot(sector, slot));
        if (p) return p;
      }
      this.hiddenHosts++;
      return null;
    }
    let key: string;
    let capacity: number;
    let toPoint: (slot: number) => Point;
    {
      const block = this.blockFor(districtId, kind);
      if (block === null) {
        this.hiddenHosts++;
        return null;
      }
      key = `block:${block}`;
      capacity = SLOTS_PER_BLOCK;
      const centre = blockCentre(block);
      toPoint = (slot) => slotCentre(centre, slot);
    }
    const p = this.claim(id, key, capacity, toPoint);
    if (!p) this.hiddenHosts++;
    return p;
  }

  /** Home slot = hash(id) mod capacity, then linear probing; null when full. */
  private claim(id: string, key: string, capacity: number, toPoint: (slot: number) => Point): Point | null {
    let occ = this.slots.get(key);
    if (!occ) {
      occ = new Array<string | null>(capacity).fill(null);
      this.slots.set(key, occ);
    }
    const home = fnv1a32(id) % capacity;
    for (let i = 0; i < capacity; i++) {
      const slot = (home + i) % capacity;
      if (occ[slot] === null) {
        occ[slot] = id;
        const point = toPoint(slot);
        this.placed.set(id, { key, slot, point });
        return point;
      }
    }
    return null;
  }

  /** Slot index of a placed host, for tests and probes. */
  slotOf(id: string): number | null {
    return this.placed.get(id)?.slot ?? null;
  }

  /** Frees a removed host's slot. */
  release(id: string): void {
    const p = this.placed.get(id);
    if (!p) return;
    const occ = this.slots.get(p.key);
    if (occ) occ[p.slot] = null;
    this.placed.delete(id);
  }

  /** Clears everything (new hello). */
  reset(): void {
    this.blockOf.clear();
    this.blockTaken.fill(null);
    this.slots.clear();
    this.placed.clear();
    this.hiddenDistricts = 0;
    this.hiddenHosts = 0;
  }

  /** Blocks in use, for drawing district plots. */
  blocks(): { districtId: string; centre: Point }[] {
    return this.blockTaken.flatMap((d, i) => (d === null ? [] : [{ districtId: d, centre: blockCentre(i) }]));
  }
}
