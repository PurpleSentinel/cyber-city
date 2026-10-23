// SPDX-License-Identifier: MIT
import * as THREE from "three";
import { describe, expect, it } from "vitest";
import { PARTICLES } from "../src/config";
import { QualityController } from "../src/render/quality";

function targets() {
  const ratios: number[] = [];
  const mat = new THREE.MeshStandardMaterial();
  return {
    ratios,
    mat,
    t: {
      renderer: { shadowMap: { enabled: false }, setPixelRatio: (r: number) => ratios.push(r) },
      moon: new THREE.DirectionalLight(),
      materials: [mat],
      particles: { capacity: 0 },
      bloom: { enabled: false },
      devicePixelRatio: 3,
    },
  };
}

describe("QualityController", () => {
  it("high: shadows, bloom, full particle pool, pixel ratio up to 2", () => {
    const { t, ratios } = targets();
    const q = new QualityController(t, "high");
    expect(q.shadows).toBe(true);
    expect(t.bloom.enabled).toBe(true);
    expect(t.particles.capacity).toBe(PARTICLES.pool);
    expect(ratios.at(-1)).toBe(2);
  });

  it("low: no shadows or bloom, half particles, pixel ratio 1; toggles back", () => {
    const { t, ratios, mat } = targets();
    const before = mat.version;
    const q = new QualityController(t, "low");
    expect(q.shadows).toBe(false);
    expect(t.bloom.enabled).toBe(false);
    expect(t.particles.capacity).toBe(PARTICLES.poolLow);
    expect(ratios.at(-1)).toBe(1);
    expect(mat.version).toBeGreaterThan(before); // recompiles for shadow change
    expect(q.toggle()).toBe("high");
    expect(q.shadows).toBe(true);
  });
});
