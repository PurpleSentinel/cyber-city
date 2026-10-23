// SPDX-License-Identifier: MIT
import { describe, expect, it } from "vitest";
import { degradedLevel, sampleEffect } from "../src/state/status";

describe("degradedLevel", () => {
  it("follows the VISUAL_SPEC formula", () => {
    expect(degradedLevel(null, 0)).toBe(0);
    expect(degradedLevel(100, 0)).toBe(0);
    expect(degradedLevel(150, 0.02)).toBe(0);
    expect(degradedLevel(325, 0)).toBeCloseTo(0.5);
    expect(degradedLevel(500, 0)).toBe(1);
    expect(degradedLevel(900, 0)).toBe(1);
    expect(degradedLevel(null, 0.06)).toBeCloseTo(0.5);
    expect(degradedLevel(325, 0.1)).toBe(1); // the worse of the two
  });
});

describe("sampleEffect", () => {
  it("flashes above 4× and 1 MB/s, strikes lightning above 10× and 10 MB/s", () => {
    expect(sampleEffect(100_000, 300_000)).toBe("none"); // 3×
    expect(sampleEffect(100_000, 500_000)).toBe("none"); // 5× but < 1 MB/s
    expect(sampleEffect(300_000, 2_000_000)).toBe("flash"); // ~6.7×, > 1 MB/s
    expect(sampleEffect(1_000_000, 11_000_000)).toBe("lightning"); // 11×, > 10 MB/s
    expect(sampleEffect(2_000_000, 15_000_000)).toBe("flash"); // 7.5×: flash only
    expect(sampleEffect(0, 2_000_000)).toBe("flash"); // from silence
  });
});
