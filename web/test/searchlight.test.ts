import * as THREE from "three";
import { describe, expect, it } from "vitest";
import type { Footprint } from "../src/render/effects/searchlight";
import { Searchlights, aimIndex } from "../src/render/effects/searchlight";

const city: Record<string, Footprint> = {
  scanner: { x: 0, z: 0, base: 0, height: 10 },
  a: { x: 50, z: 0, base: 0, height: 10 },
  b: { x: 0, z: 50, base: 0, height: 10 },
  c: { x: -50, z: 0, base: 0, height: 10 },
};
const lookup = (id: string): Footprint | null => city[id] ?? null;

describe("aimIndex", () => {
  it("visits each target for 0.3 s, loops, and stops at 6 s", () => {
    expect(aimIndex(0, 3)).toBe(0);
    expect(aimIndex(0.29, 3)).toBe(0);
    expect(aimIndex(0.31, 3)).toBe(1);
    expect(aimIndex(0.61, 3)).toBe(2);
    expect(aimIndex(0.91, 3)).toBe(0); // loops when time remains
    expect(aimIndex(5.99, 3)).toBe(19 % 3);
    expect(aimIndex(6, 3)).toBe(-1);
    expect(aimIndex(1, 0)).toBe(-1);
  });
});

describe("Searchlights", () => {
  it("sweeps targets in order, outlines them, and ends after 6 s", () => {
    const s = new Searchlights(lookup);
    expect(s.start("scanner", ["a", "b", "c"])).toBe(true);
    expect(s.aimedAt("scanner")).toBe("a");
    const cone = s.group.children[0] as THREE.Mesh;
    const axis = new THREE.Vector3(0, -1, 0).applyQuaternion(cone.quaternion);
    expect(axis.x).toBeGreaterThan(0.9); // aims at a (+x)
    expect(cone.position.y).toBe(10); // apex on the roof
    expect(s.group.children.filter((c) => c instanceof THREE.LineSegments)).toHaveLength(3);
    s.frame(0.35);
    expect(s.aimedAt("scanner")).toBe("b");
    s.frame(0.3);
    expect(s.aimedAt("scanner")).toBe("c");
    s.frame(5.4);
    expect(s.active).toBe(0);
    expect(s.group.children).toHaveLength(0);
  });

  it("caps concurrent searchlights at 3 and lets the newest own a shared outline", () => {
    const s = new Searchlights(lookup);
    expect(s.start("scanner", ["a"])).toBe(true);
    expect(s.start("b", ["a"])).toBe(true);
    expect(s.group.children.filter((c) => c instanceof THREE.LineSegments)).toHaveLength(1);
    expect(s.start("c", ["b"])).toBe(true);
    expect(s.start("a", ["c"])).toBe(false);
    expect(s.active).toBe(3);
  });

  it("does not start for a scanner that is not drawn", () => {
    expect(new Searchlights(lookup).start("missing", ["a"])).toBe(false);
  });
});
