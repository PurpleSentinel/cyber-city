// SPDX-License-Identifier: MIT
import * as THREE from "three";
import { describe, expect, it } from "vitest";
import { Lightning } from "../src/render/effects/lightning";

describe("Lightning", () => {
  it("strikes from y = 200 to the roof, re-jitters, and ends after 0.4 s", () => {
    let n = 0;
    const l = new Lightning(() => (n++ % 10) / 10);
    l.strike(10, -20, 31);
    expect(l.active).toBe(1);
    const line = l.group.children[0] as THREE.Line;
    const pos = line.geometry.getAttribute("position") as THREE.BufferAttribute;
    expect(pos.getY(0)).toBe(200);
    expect(pos.getY(3)).toBe(31);
    expect([pos.getX(3), pos.getZ(3)]).toEqual([10, -20]); // ends exactly on the roof
    const before = pos.getX(1);
    l.frame(0.06); // > 50 ms: new jagged path
    expect(pos.getX(1)).not.toBe(before);
    l.frame(0.35);
    expect(l.active).toBe(0);
    expect(l.group.children).toHaveLength(0);
  });
});
