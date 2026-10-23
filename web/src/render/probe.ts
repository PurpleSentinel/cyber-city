// Screen-space probes and fixed camera poses for the e2e shading test
// (docs/VISUAL_SPEC.md "Debug hook", TESTING.md "Shading test").
import * as THREE from "three";
import { LAYOUT } from "../config";
import type { ProbeResult } from "../debug";
import type { Point } from "../layout/layout";

export interface BuildingGeometry {
  x: number;
  z: number;
  base: number;
  height: number;
  local: boolean;
}

const v = new THREE.Vector3();

function toScreen(camera: THREE.Camera, w: number, h: number, x: number, y: number, z: number): [number, number] {
  v.set(x, y, z).project(camera);
  return [((v.x + 1) / 2) * w, ((1 - v.y) / 2) * h];
}

/** Which plot (block centre) a ground point lies on, or null for bare ground. */
function plotAt(blocks: readonly Point[], x: number, z: number): Point | null {
  const half = LAYOUT.blockSize / 2;
  return blocks.find((c) => Math.abs(x - c.x) <= half && Math.abs(z - c.z) <= half) ?? null;
}

/**
 * Face corners of the +x and +z faces, plus ground points inside and outside
 * the cast shadow (shadow falls toward (−0.7, +0.3), VISUAL_SPEC "Light rig").
 */
export function probe(
  g: BuildingGeometry,
  camera: THREE.Camera,
  width: number,
  height: number,
  blocks: readonly Point[],
): ProbeResult {
  const top = g.base + g.height;
  const s = (x: number, y: number, z: number) => toScreen(camera, width, height, x, y, z);
  const px: [number, number][] = [s(g.x + 1, g.base, g.z - 1), s(g.x + 1, g.base, g.z + 1), s(g.x + 1, top, g.z + 1), s(g.x + 1, top, g.z - 1)];
  const pz: [number, number][] = [s(g.x - 1, g.base, g.z + 1), s(g.x + 1, g.base, g.z + 1), s(g.x + 1, top, g.z + 1), s(g.x - 1, top, g.z + 1)];
  const sh = { x: g.x - 0.35 * g.height, z: g.z + 0.15 * g.height };
  const lt = { x: g.x + 0.15 * g.height, z: g.z + 0.35 * g.height };
  const a = plotAt(blocks, sh.x, sh.z);
  const b = plotAt(blocks, lt.x, lt.z);
  return {
    px,
    pz,
    shadow: s(sh.x, g.base, sh.z),
    lit: s(lt.x, g.base, lt.z),
    sameSurface: a === b,
  };
}

/** Camera position and target that frame a building's +x and +z faces. */
export function poseFor(g: BuildingGeometry): { position: THREE.Vector3; target: THREE.Vector3 } {
  const d = Math.max(14, 0.9 * g.height);
  return {
    position: new THREE.Vector3(g.x + d, g.base + 0.6 * g.height + 6, g.z + d),
    target: new THREE.Vector3(g.x, g.base + g.height / 2, g.z),
  };
}
