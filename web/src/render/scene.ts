// SPDX-License-Identifier: MIT
// Lighting rig, ground and district plots (docs/VISUAL_SPEC.md "Lighting,
// materials and shading"). The city is solid and lit, never wireframe (D17).
import * as THREE from "three";
import { LAYOUT, LIGHTING, MATERIALS, PALETTE, SCENE } from "../config";
import type { Point } from "../layout/layout";

export type Quality = "high" | "low";

/** Tone mapping, colour space and shadow settings for the quality mode. */
export function configureRenderer(r: THREE.WebGLRenderer, quality: Quality): void {
  r.toneMapping = THREE.ACESFilmicToneMapping;
  r.toneMappingExposure = LIGHTING.toneMappingExposure;
  r.outputColorSpace = THREE.SRGBColorSpace;
  r.shadowMap.enabled = quality === "high";
  r.shadowMap.type = THREE.PCFShadowMap; // PCFSoftShadowMap was removed in three r18x
}

export interface Lights {
  hemi: THREE.HemisphereLight;
  moon: THREE.DirectionalLight;
}

/** Hemisphere sky light plus the shadow-casting moon. */
export function addLights(scene: THREE.Scene, quality: Quality): Lights {
  const h = LIGHTING.hemisphere;
  const hemi = new THREE.HemisphereLight(h.sky, h.ground, h.intensity);
  scene.add(hemi);

  const m = LIGHTING.moon;
  const moon = new THREE.DirectionalLight(m.color, m.intensity);
  moon.position.set(...m.direction).normalize().multiplyScalar(m.distance);
  moon.target.position.set(0, 0, 0);
  const s = LIGHTING.shadow;
  moon.castShadow = quality === "high";
  moon.shadow.mapSize.set(s.mapSize, s.mapSize);
  Object.assign(moon.shadow.camera, { left: -s.extent, right: s.extent, top: s.extent, bottom: -s.extent, near: s.near, far: s.far });
  moon.shadow.camera.updateProjectionMatrix();
  moon.shadow.bias = s.bias;
  moon.shadow.normalBias = s.normalBias;
  scene.add(moon, moon.target);
  return { hemi, moon };
}

/** Procedural environment for soft reflections on the glass (no external HDR). */
export function setEnvironment(scene: THREE.Scene, texture: THREE.Texture): void {
  scene.environment = texture;
  scene.environmentIntensity = LIGHTING.environment.intensity;
}

export function groundMaterial(): THREE.MeshStandardMaterial {
  const g = MATERIALS.ground;
  return new THREE.MeshStandardMaterial({ color: g.color, roughness: g.roughness, metalness: g.metalness });
}

/** Lit ground plane plus a translucent grid overlay (lines allowed only here). */
export function createGround(): THREE.Group {
  const group = new THREE.Group();
  const plane = new THREE.Mesh(new THREE.PlaneGeometry(SCENE.groundSize, SCENE.groundSize), groundMaterial());
  plane.rotation.x = -Math.PI / 2;
  plane.receiveShadow = true;
  plane.name = "ground";
  group.add(plane);

  const grid = new THREE.GridHelper(SCENE.groundSize, SCENE.gridDivisions, PALETTE.gridMajor, PALETTE.gridMinor);
  grid.position.y = SCENE.gridY;
  const mats = Array.isArray(grid.material) ? grid.material : [grid.material];
  for (const [i, mat] of mats.entries()) {
    mat.transparent = true;
    mat.depthWrite = false;
    mat.opacity = i === 0 ? SCENE.gridMajorOpacity : SCENE.gridMinorOpacity;
  }
  group.add(grid);
  return group;
}

/**
 * District plot material: lit slab with an emissive border strip drawn in the
 * shader, so all plots stay one instanced draw call.
 */
export function plotMaterial(): THREE.MeshStandardMaterial {
  const p = MATERIALS.plot;
  const mat = new THREE.MeshStandardMaterial({ color: p.color, roughness: p.roughness, metalness: p.metalness });
  const half = LAYOUT.blockSize / 2;
  mat.onBeforeCompile = (shader) => {
    shader.uniforms.uBorder = { value: new THREE.Color(PALETTE.hud).multiplyScalar(p.borderIntensity) };
    shader.vertexShader = shader.vertexShader
      .replace("#include <common>", "#include <common>\nvarying vec3 vPlotLocal;")
      .replace("#include <begin_vertex>", "#include <begin_vertex>\nvPlotLocal = position;");
    shader.fragmentShader = shader.fragmentShader
      .replace("#include <common>", "#include <common>\nvarying vec3 vPlotLocal;\nuniform vec3 uBorder;")
      .replace(
        "#include <emissivemap_fragment>",
        `#include <emissivemap_fragment>
        float edge = step(${(half - p.borderWidth).toFixed(3)}, max(abs(vPlotLocal.x), abs(vPlotLocal.z)));
        totalEmissiveRadiance += uBorder * edge;`,
      );
  };
  return mat;
}

/** One plot per active district block (at most 9). */
export class Plots {
  readonly mesh: THREE.InstancedMesh;
  private readonly dummy = new THREE.Object3D();

  constructor() {
    const geo = new THREE.BoxGeometry(LAYOUT.blockSize, MATERIALS.plot.height, LAYOUT.blockSize);
    geo.translate(0, MATERIALS.plot.height / 2, 0);
    this.mesh = new THREE.InstancedMesh(geo, plotMaterial(), LAYOUT.gridBlocks * LAYOUT.gridBlocks);
    this.mesh.receiveShadow = true;
    this.mesh.count = 0;
    this.mesh.name = "plots";
  }

  update(blocks: readonly { centre: Point }[]): void {
    this.mesh.count = blocks.length;
    blocks.forEach((b, i) => {
      this.dummy.position.set(b.centre.x, 0, b.centre.z);
      this.dummy.updateMatrix();
      this.mesh.setMatrixAt(i, this.dummy.matrix);
    });
    this.mesh.instanceMatrix.needsUpdate = true;
  }
}
