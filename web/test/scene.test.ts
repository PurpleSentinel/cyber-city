// SPDX-License-Identifier: MIT
// Shading guards (docs/TESTING.md "Shading guards"): lit, filled materials and
// the documented light rig. three.js scene objects work without WebGL.
import * as THREE from "three";
import { describe, expect, it } from "vitest";
import { Plots, addLights, createGround, groundMaterial, plotMaterial, setEnvironment } from "../src/render/scene";

describe("scene shading guards", () => {
  it("uses lit, filled materials for ground and plots", () => {
    for (const m of [groundMaterial(), plotMaterial()]) {
      expect(m).toBeInstanceOf(THREE.MeshStandardMaterial);
      expect(m.wireframe).toBe(false);
    }
    const plane = createGround().getObjectByName("ground") as THREE.Mesh;
    expect(plane.receiveShadow).toBe(true);
    expect(new Plots().mesh.receiveShadow).toBe(true);
  });

  it("has exactly one hemisphere light and one shadow-casting moon in high quality", () => {
    const scene = new THREE.Scene();
    addLights(scene, "high");
    setEnvironment(scene, new THREE.Texture());
    const hemis: THREE.HemisphereLight[] = [];
    const dirs: THREE.DirectionalLight[] = [];
    scene.traverse((o) => {
      if (o instanceof THREE.HemisphereLight) hemis.push(o);
      if (o instanceof THREE.DirectionalLight) dirs.push(o);
    });
    expect(hemis).toHaveLength(1);
    expect(dirs).toHaveLength(1);
    expect(dirs[0]!.castShadow).toBe(true);
    expect(scene.environment).not.toBeNull();

    // The moon lights +x faces and not +z faces (VISUAL_SPEC "Light rig").
    const dir = dirs[0]!.position.clone().normalize();
    expect(dir.x).toBeGreaterThan(0.4);
    expect(dir.z).toBeLessThan(0);
  });

  it("drops shadows but keeps lights in low quality", () => {
    const scene = new THREE.Scene();
    const { moon } = addLights(scene, "low");
    expect(moon.castShadow).toBe(false);
  });

  it("places one plot per active block", () => {
    const plots = new Plots();
    plots.update([{ centre: { x: 0, z: 0 } }, { centre: { x: 80, z: -80 } }]);
    expect(plots.mesh.count).toBe(2);
    const m = new THREE.Matrix4();
    plots.mesh.getMatrixAt(1, m);
    expect(new THREE.Vector3().setFromMatrixPosition(m).toArray()).toEqual([80, 0, -80]);
  });
});
