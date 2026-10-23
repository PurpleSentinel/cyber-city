// SPDX-License-Identifier: MIT
// Quality modes (docs/VISUAL_SPEC.md "Scene and camera", key L): high has
// bloom and shadows; low drops bloom and shadows, halves particles and uses
// pixel ratio 1. Materials stay lit in both (D17).
import * as THREE from "three";
import { EffectComposer } from "three/examples/jsm/postprocessing/EffectComposer.js";
import { OutputPass } from "three/examples/jsm/postprocessing/OutputPass.js";
import { RenderPass } from "three/examples/jsm/postprocessing/RenderPass.js";
import { UnrealBloomPass } from "three/examples/jsm/postprocessing/UnrealBloomPass.js";
import { BLOOM, PARTICLES, SCENE } from "../config";
import type { Quality } from "./scene";

/** The parts of the renderer the controller touches (structural for tests). */
export interface QualityRenderer {
  shadowMap: { enabled: boolean };
  setPixelRatio(r: number): void;
}

export interface QualityTargets {
  renderer: QualityRenderer;
  moon: THREE.DirectionalLight;
  /** Lit materials that must recompile when shadows toggle. */
  materials: THREE.Material[];
  particles: { capacity: number };
  bloom: { enabled: boolean } | null;
  devicePixelRatio: number;
}

export class QualityController {
  constructor(
    private readonly t: QualityTargets,
    public mode: Quality,
  ) {
    this.apply(mode);
  }

  get shadows(): boolean {
    return this.t.renderer.shadowMap.enabled && this.t.moon.castShadow;
  }

  apply(mode: Quality): void {
    this.mode = mode;
    const high = mode === "high";
    this.t.renderer.shadowMap.enabled = high;
    this.t.moon.castShadow = high;
    for (const m of this.t.materials) m.needsUpdate = true; // shadow chunks change
    this.t.particles.capacity = high ? PARTICLES.pool : PARTICLES.poolLow;
    if (this.t.bloom) this.t.bloom.enabled = high;
    this.t.renderer.setPixelRatio(high ? Math.min(this.t.devicePixelRatio, SCENE.maxPixelRatio) : 1);
  }

  toggle(): Quality {
    this.apply(this.mode === "high" ? "low" : "high");
    return this.mode;
  }
}

/** RenderPass → UnrealBloomPass → OutputPass (tone mapping and sRGB). */
export function createComposer(renderer: THREE.WebGLRenderer, scene: THREE.Scene, camera: THREE.Camera) {
  const composer = new EffectComposer(renderer);
  composer.addPass(new RenderPass(scene, camera));
  const size = renderer.getSize(new THREE.Vector2());
  const bloom = new UnrealBloomPass(size, BLOOM.strength, BLOOM.radius, BLOOM.threshold);
  composer.addPass(bloom);
  composer.addPass(new OutputPass());
  return { composer, bloom };
}
