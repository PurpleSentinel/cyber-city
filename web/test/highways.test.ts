import { describe, expect, it } from "vitest";
import type { Flow } from "../src/protocol";
import { HighwayModel, pairKey, roadOpacity, roadWidth, route } from "../src/state/highways";

const flow = (src: string, dst: string, bytes: number, category = "https") => ({ src, dst, bytes, category }) as Flow;
const all = () => true;

describe("roadWidth", () => {
  it("follows clamp(0.2 + 0.6·log10(1 + rate/1000), 0.2, 3)", () => {
    expect(roadWidth(0)).toBe(0.2);
    expect(roadWidth(9_000)).toBeCloseTo(0.8, 5); // log10(10) = 1
    expect(roadWidth(99_000)).toBeCloseTo(1.4, 5);
    expect(roadWidth(1e12)).toBe(3);
  });
});

describe("route", () => {
  it("is L-shaped (x first, then z) between local buildings and straight otherwise", () => {
    const a = { x: 0, z: 0 };
    const b = { x: 10, z: 20 };
    expect(route(a, b, true)).toEqual([
      { from: a, to: { x: 10, z: 0 } },
      { from: { x: 10, z: 0 }, to: b },
    ]);
    expect(route(a, b, false)).toEqual([{ from: a, to: b }]);
    expect(route(a, { x: 0, z: 5 }, true)).toHaveLength(1); // already aligned
  });
});

describe("HighwayModel", () => {
  it("pairs both directions into one road with the dominant category", () => {
    const m = new HighwayModel();
    m.applyTick([flow("a", "b", 1000, "dns"), flow("b", "a", 5000, "https"), flow("a", "224.0.0.251", 999)], 100, 1000, (id) => id !== "224.0.0.251");
    const roads = m.roads();
    expect(roads).toHaveLength(1);
    expect(roads[0]!.key).toBe(pairKey("b", "a"));
    expect(roads[0]!.category).toBe("https");
    expect(roads[0]!.width).toBeCloseTo(roadWidth(60_000), 5);
  });

  it("fades over 3 s after the last packet, then frees the road", () => {
    expect(roadOpacity(0)).toBeCloseTo(0.35);
    expect(roadOpacity(1500)).toBeCloseTo(0.175);
    expect(roadOpacity(3000)).toBe(0);
    const m = new HighwayModel();
    m.applyTick([flow("a", "b", 1000)], 100, 1000, all);
    expect(m.roads(2500)[0]!.opacity).toBeCloseTo(0.175);
    expect(m.roads(4000)).toHaveLength(0);
    expect(m.roads(1000)).toHaveLength(0); // freed, not just hidden
  });

  it("keeps only the 500 busiest roads", () => {
    const m = new HighwayModel();
    const flows = Array.from({ length: 600 }, (_, i) => flow(`h${i}`, "hub", (i + 1) * 10));
    m.applyTick(flows, 100, 1000, all);
    const roads = m.roads();
    expect(roads).toHaveLength(500);
    expect(roads[0]!.a).toBe("h599"); // busiest first
    expect(roads.some((r) => r.a === "h0")).toBe(false);
  });

  it("drops roads of removed hosts", () => {
    const m = new HighwayModel();
    m.applyTick([flow("a", "b", 10), flow("c", "d", 10)], 100, 1000, all);
    m.removeHost("a");
    expect(m.roads().map((r) => r.key)).toEqual(["c|d"]);
  });
});
