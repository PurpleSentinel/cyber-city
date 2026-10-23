import * as THREE from "three";
import { describe, expect, it } from "vitest";
import { PALETTE } from "../src/config";
import { Meteors, skyMix } from "../src/render/effects/meteor";
import type { Footprint } from "../src/render/effects/searchlight";

const city: Record<string, Footprint> = {
  web: { x: 10, z: -10, base: 0.4, height: 20 },
  db: { x: -30, z: 5, base: 0.4, height: 8 },
  dns: { x: 0, z: 40, base: 0.4, height: 4 },
};
const lookup = (id: string): Footprint | null => city[id] ?? null;

describe("skyMix", () => {
  it("ramps in over 1 s, holds, and returns over 2 s", () => {
    expect(skyMix(0)).toBe(0);
    expect(skyMix(0.5)).toBeCloseTo(0.5);
    expect(skyMix(1)).toBe(1);
    expect(skyMix(5)).toBe(1);
    expect(skyMix(6)).toBe(1);
    expect(skyMix(7)).toBeCloseTo(0.5);
    expect(skyMix(8)).toBe(0);
  });
});

describe("Meteors", () => {
  it("lerps the sky colour along the timeline and cleans up after 8 s", () => {
    const m = new Meteors(lookup);
    const bg = new THREE.Color(PALETTE.bg);
    const alert = new THREE.Color(PALETTE.alertSky);
    expect(m.start("web")).toBe(true);
    expect(m.skyColor().equals(bg)).toBe(true); // t = 0
    m.frame(1);
    expect(m.skyColor().getHex()).toBe(alert.getHex()); // t = 1
    m.frame(4);
    expect(m.skyColor().getHex()).toBe(alert.getHex()); // t = 5
    m.frame(3);
    expect(m.active).toBe(0); // t = 8
    expect(m.skyColor().equals(bg)).toBe(true);
    expect(m.group.children).toHaveLength(0);
  });

  it("drops particles from the sky onto the target roof", () => {
    const m = new Meteors(lookup);
    m.start("web", true);
    const points = m.group.children.find((c) => c instanceof THREE.Points) as THREE.Points;
    const pos = points.geometry.getAttribute("position") as THREE.BufferAttribute;
    expect(pos.count).toBe(200); // low quality
    for (let i = 0; i < pos.count; i++) {
      expect(pos.getY(i)).toBeLessThanOrEqual(200);
      expect(pos.getY(i)).toBeGreaterThanOrEqual(20.4 - 1e-6);
      expect(Math.hypot(pos.getX(i) - 10, pos.getZ(i) + 10)).toBeLessThanOrEqual(120);
    }
    const shield = m.group.children.find((c) => c instanceof THREE.Mesh) as THREE.Mesh;
    expect((shield.geometry as THREE.IcosahedronGeometry).parameters.radius).toBe(12); // max(6, 20 · 0.6)
    m.frame(0.1);
    const opacity = (shield.material as THREE.MeshBasicMaterial).opacity;
    expect(opacity).toBeGreaterThanOrEqual(0.1);
    expect(opacity).toBeLessThanOrEqual(0.6);
  });

  it("runs at most 2 showers", () => {
    const m = new Meteors(lookup);
    expect(m.start("web")).toBe(true);
    expect(m.start("db")).toBe(true);
    expect(m.start("dns")).toBe(false);
    expect(m.start("missing")).toBe(false);
  });
});
