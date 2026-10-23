// SPDX-License-Identifier: MIT
import { describe, expect, it } from "vitest";
import { ParticleSystem, along, emissionRate, travelTime } from "../src/state/particles";

describe("particle formulas", () => {
  it("emits clamp(1 + log2(1 + pps), 1, 20) per second", () => {
    expect(emissionRate(0)).toBe(1);
    expect(emissionRate(1)).toBe(2);
    expect(emissionRate(7)).toBe(4);
    expect(emissionRate(1e9)).toBe(20);
  });

  it("travels in clamp(0.4 + rtt·0.02, 0.4, 4) s, 1.5 s when unknown", () => {
    expect(travelTime(null)).toBe(1.5);
    expect(travelTime(0)).toBe(0.4);
    expect(travelTime(12)).toBeCloseTo(0.64);
    expect(travelTime(500)).toBe(4);
  });

  it("moves along an L-shaped path", () => {
    const path = [{ x: 0, z: 0 }, { x: 10, z: 0 }, { x: 10, z: 10 }];
    expect(along(path, 0).p).toEqual({ x: 0, z: 0 });
    expect(along(path, 0.25).p).toEqual({ x: 5, z: 0 });
    expect(along(path, 0.75)).toEqual({ p: { x: 10, z: 5 }, heading: Math.PI / 2 });
    expect(along(path, 1).p).toEqual({ x: 10, z: 10 });
  });
});

describe("ParticleSystem", () => {
  const emitter = (key: string, rate: number) => ({ key, path: [{ x: 0, z: 0 }, { x: 1, z: 0 }], rate, duration: 1, category: "https" });

  it("emits at the emitter rate and retires particles after their journey", () => {
    const s = new ParticleSystem(100);
    for (let i = 0; i < 10; i++) s.step(0.1, [emitter("r", 10)]);
    expect(s.live).toHaveLength(10);
    s.step(1.0, []);
    expect(s.live).toHaveLength(0);
  });

  it("never exceeds the pool", () => {
    const s = new ParticleSystem(50);
    const many = Array.from({ length: 400 }, (_, i) => emitter(`r${i}`, 20));
    for (let i = 0; i < 30; i++) {
      s.step(0.05, many);
      expect(s.live.length).toBeLessThanOrEqual(50);
    }
    expect(s.live).toHaveLength(50);
  });
});
