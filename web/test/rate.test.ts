// SPDX-License-Identifier: MIT
import { describe, expect, it } from "vitest";
import type { Flow } from "../src/protocol";
import { addSample, bytesByHost, decayedRate, newRate, stepHeight, targetHeight } from "../src/state/rate";

describe("targetHeight", () => {
  it("matches the VISUAL_SPEC examples", () => {
    expect(targetHeight(0)).toBe(1);
    expect(targetHeight(1_000)).toBeCloseTo(2.8, 1);
    expect(targetHeight(1_000_000)).toBeCloseTo(19, 0);
    expect(targetHeight(100_000_000)).toBeCloseTo(31, 0);
    expect(targetHeight(1e15)).toBe(40);
  });
});

describe("rate EWMA", () => {
  it("takes the first sample as the rate", () => {
    const s = newRate();
    expect(addSample(s, 100_000, 100, 1_000)).toBe(1_000_000);
    expect(s.rate).toBe(1_000_000);
  });

  it("moves halfway toward a new sample after one half-life", () => {
    const s = newRate();
    addSample(s, 100_000, 100, 1_000); // 1 MB/s
    addSample(s, 0, 100, 3_000); // 2 s later: half-life 2 s
    expect(s.rate).toBeCloseTo(500_000, 0);
  });

  it("decays between samples, halving per half-life", () => {
    const s = newRate();
    addSample(s, 100_000, 100, 1_000);
    expect(decayedRate(s, 1_000)).toBe(1_000_000);
    expect(decayedRate(s, 3_000)).toBeCloseTo(500_000, 0);
    expect(decayedRate(s, 5_000)).toBeCloseTo(250_000, 0);
    expect(decayedRate(newRate(), 5_000)).toBe(0);
  });

  it("brings a quiet building's height down toward 1", () => {
    const s = newRate();
    addSample(s, 100_000, 100, 1_000);
    const tall = targetHeight(decayedRate(s, 1_000));
    const later = targetHeight(decayedRate(s, 21_000)); // 10 half-lives
    expect(tall).toBeGreaterThan(18);
    expect(later).toBeLessThan(tall / 2);
    expect(targetHeight(decayedRate(s, 61_000))).toBeLessThan(1.1);
  });
});

describe("stepHeight", () => {
  it("uses frame dt, not a fixed per-frame factor", () => {
    // One 0.25 s step equals five 0.05 s steps (time constant 0.25 s).
    let h = 1;
    for (let i = 0; i < 5; i++) h = stepHeight(h, 11, 0.05);
    expect(h).toBeCloseTo(stepHeight(1, 11, 0.25), 10);
    expect(stepHeight(1, 11, 0.25)).toBeCloseTo(1 + 10 * (1 - Math.exp(-1)), 10);
    expect(stepHeight(5, 5, 1)).toBe(5);
  });
});

describe("bytesByHost", () => {
  it("credits both endpoints of each flow", () => {
    const f = (src: string, dst: string, bytes: number) => ({ src, dst, bytes }) as Flow;
    const m = bytesByHost([f("a", "b", 100), f("a", "c", 50), f("c", "c", 7)]);
    expect(Object.fromEntries(m)).toEqual({ a: 150, b: 100, c: 57 });
  });
});
