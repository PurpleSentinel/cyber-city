import { describe, expect, it } from "vitest";
import { LAYOUT } from "../src/config";
import { fnv1a32 } from "../src/layout/hash";
import type { Point } from "../src/layout/layout";
import { Layout, RINGS, blockCentre, outlandsSlot, sectorFor } from "../src/layout/layout";

// Deterministic pseudo-random ids (no Math.random in tests).
function ids(n: number, seed: number, prefix: string | null = "10.0"): string[] {
  let x = seed;
  const next = () => (x = (Math.imul(x, 1103515245) + 12345) >>> 0);
  return Array.from({ length: n }, () => {
    const p = prefix ?? `${next() & 255}.${next() & 255}`; // null: varied /16s
    next();
    return `${p}.${(x >>> 8) & 255}.${x & 255}`;
  });
}

const overlaps = (a: Point, b: Point) =>
  Math.abs(a.x - b.x) < LAYOUT.footprint && Math.abs(a.z - b.z) < LAYOUT.footprint;

describe("fnv1a32", () => {
  it("matches the reference FNV-1a values", () => {
    expect(fnv1a32("")).toBe(0x811c9dc5);
    expect(fnv1a32("a")).toBe(0xe40c292c);
    expect(fnv1a32("foobar")).toBe(0xbf9cf968);
  });
});

describe("Layout", () => {
  it("puts server districts in the centre block first", () => {
    const l = new Layout();
    expect(l.blockFor("core", "server")).toBe(4);
    expect(l.blockFor("dmz", "server")).toBe(1);
    expect(blockCentre(4)).toEqual({ x: 0, z: 0 });
  });

  it("hides districts beyond nine", () => {
    const l = new Layout();
    const blocks = Array.from({ length: 10 }, (_, i) => l.blockFor(`lan-${i}`, "lan"));
    expect(new Set(blocks.filter((b) => b !== null)).size).toBe(9);
    expect(blocks.filter((b) => b === null)).toHaveLength(1);
    expect(l.hiddenDistricts).toBe(1);
  });

  it("never overlaps 2000 buildings", () => {
    const l = new Layout();
    const points: Point[] = [];
    const local = ids(200, 1);
    local.forEach((id, i) => {
      const p = l.place(id, `lan-${i % 5}`, i % 5 === 0 ? "server" : "lan");
      if (p) points.push(p);
    });
    for (const id of ids(1800, 2, null)) {
      const p = l.place(id, "outlands", "outlands");
      if (p) points.push(p);
    }
    expect(points).toHaveLength(2000);
    // Bucket by coarse cell to keep the overlap check fast.
    const grid = new Map<string, Point[]>();
    for (const p of points) {
      const k = `${Math.floor(p.x / 4)},${Math.floor(p.z / 4)}`;
      for (let dx = -1; dx <= 1; dx++)
        for (let dz = -1; dz <= 1; dz++) {
          for (const q of grid.get(`${Math.floor(p.x / 4) + dx},${Math.floor(p.z / 4) + dz}`) ?? []) {
            expect(overlaps(p, q)).toBe(false);
          }
        }
      grid.set(k, [...(grid.get(k) ?? []), p]);
    }
  });

  it("lands each host in its home slot unless taken; same order, same city (D24)", () => {
    const hosts = ids(60, 7);
    const a = new Layout();
    const b = new Layout();
    const taken = new Set<number>();
    for (const id of hosts) {
      const home = fnv1a32(id) % 256;
      a.place(id, "corp", "lan");
      if (!taken.has(home)) expect(a.slotOf(id)).toBe(home);
      taken.add(a.slotOf(id)!);
    }
    for (const id of [...hosts].reverse()) b.place(id, "corp", "lan");
    const atHome = hosts.filter((id) => a.slotOf(id) === fnv1a32(id) % 256 && b.slotOf(id) === fnv1a32(id) % 256);
    expect(atHome.length / hosts.length).toBeGreaterThan(0.7); // order-independent for most (75% at this density)
    const c = new Layout();
    for (const id of hosts) {
      c.place(id, "corp", "lan");
      expect(c.slotOf(id)).toBe(a.slotOf(id));
    }
  });

  it("spills a full sector into its neighbours instead of hiding buildings", () => {
    const l = new Layout();
    const all = ids(1500, 9, "203.0"); // one /16 → one home sector
    const placed = all.filter((id) => l.place(id, "outlands", "outlands") !== null);
    expect(placed).toHaveLength(1500);
    expect(l.hiddenHosts).toBe(0);
  });

  it("keeps a placed building where it is and frees slots on release", () => {
    const l = new Layout();
    const p = l.place("10.0.1.10", "corp", "lan");
    expect(l.place("10.0.1.10", "corp", "lan")).toEqual(p);
    l.release("10.0.1.10");
    expect(l.place("10.0.1.10", "corp", "lan")).toEqual(p); // home slot again
  });

  it("places the Outlands on rings in the right compass sector", () => {
    expect(RINGS[0]!.radius).toBe(180);
    expect(RINGS.at(-1)!.radius).toBe(260);
    const mid = Math.floor(RINGS[0]!.perSector / 2);
    const north = outlandsSlot(0, mid);
    const east = outlandsSlot(2, mid);
    expect(Math.hypot(north.x, north.z)).toBeCloseTo(180, 5);
    expect(north.z).toBeLessThan(-179); // N is −z
    expect(east.x).toBeGreaterThan(179); // E is +x (clockwise)
    expect(sectorFor("203.0.113.10", "NE")).toBe(1);
    expect(sectorFor("203.0.113.10", null)).toBe(sectorFor("203.0.9.9", null)); // same /16
    expect(sectorFor("anon-1234abcd", null)).toBe(fnv1a32("anon-1234abcd") % 8);

    const l = new Layout();
    const p = l.place("198.51.100.20", "outlands", "outlands", "W")!;
    const deg = ((Math.atan2(p.x, -p.z) * 180) / Math.PI + 360) % 360;
    expect(deg).toBeGreaterThanOrEqual(247.5);
    expect(deg).toBeLessThan(292.5);
  });
});
