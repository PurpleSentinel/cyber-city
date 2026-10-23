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

/** Margin around plot edges where the emissive border strip spoils a sample. */
const EDGE_MARGIN = 1.5;

/**
 * Which plot (block centre) a ground point lies on, null for bare ground, or
 * "edge" within EDGE_MARGIN of a plot border.
 */
function plotAt(blocks: readonly Point[], x: number, z: number): Point | null | "edge" {
  const half = LAYOUT.blockSize / 2;
  for (const c of blocks) {
    const d = Math.max(Math.abs(x - c.x), Math.abs(z - c.z));
    if (Math.abs(d - half) < EDGE_MARGIN) return "edge";
    if (d <= half) return c;
  }
  return null;
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
  const { shadow: sh, lit: lt } = samplePoints(g);
  const a = plotAt(blocks, sh.x, sh.z);
  const b = plotAt(blocks, lt.x, lt.z);
  return {
    px,
    pz,
    shadow: s(sh.x, g.base, sh.z),
    lit: s(lt.x, g.base, lt.z),
    sameSurface: a !== "edge" && b !== "edge" && a === b,
    ring: groundRing(g, s),
  };
}

/** Ground sample points (world) for a building: inside and outside its own shadow. */
export function samplePoints(g: BuildingGeometry): { shadow: Point; lit: Point } {
  return {
    // On the shadow's centre line at 0.7h (well clear of the wall), and its
    // mirror on the camera side at the same distance.
    shadow: { x: g.x - 0.49 * g.height, z: g.z + 0.21 * g.height },
    lit: { x: g.x + 0.21 * g.height, z: g.z + 0.49 * g.height },
  };
}

/** Shadows fall toward (−0.7, +0.3) per unit of height (moon (0.7, 1, −0.3)). */
const SHADOW_DIR = { x: -0.7, z: 0.3 };

/** Whether ground point p lies in the shadow of building b. */
export function inShadowOf(p: Point, b: BuildingGeometry): boolean {
  const dx = p.x - b.x;
  const dz = p.z - b.z;
  const k = (dx * SHADOW_DIR.x + dz * SHADOW_DIR.z) / (SHADOW_DIR.x ** 2 + SHADOW_DIR.z ** 2);
  if (k < 0 || k > b.height) return false;
  return Math.abs(dx - k * SHADOW_DIR.x) <= 1 && Math.abs(dz - k * SHADOW_DIR.z) <= 1;
}

/**
 * Whether a building's sample points are clean for the shading test: neither
 * lies on another building, and the lit point is not in another's shadow.
 */
export function clearSamples(g: BuildingGeometry, others: readonly BuildingGeometry[]): boolean {
  const { shadow, lit } = samplePoints(g);
  const near = (p: Point, b: BuildingGeometry) => Math.max(Math.abs(p.x - b.x), Math.abs(p.z - b.z)) <= 2.5;
  // The pose camera must see both face centres and both ground samples.
  const { position: cam } = poseFor(g);
  const mid = g.base + g.height / 2;
  const targets: [number, number, number][] = [
    [g.x + 1, mid, g.z],
    [g.x, mid, g.z + 1],
    [shadow.x, g.base, shadow.z],
    [lit.x, g.base, lit.z],
  ];
  const blocks = (b: BuildingGeometry) =>
    targets.some(([tx, ty, tz]) => {
      for (let i = 1; i < 60; i++) {
        const t = i / 60;
        const x = cam.x + (tx - cam.x) * t;
        const y = cam.y + (ty - cam.y) * t;
        const z = cam.z + (tz - cam.z) * t;
        if (Math.abs(x - b.x) <= 1 && Math.abs(z - b.z) <= 1 && y <= b.base + b.height) return true;
      }
      return false;
    });
  return others.every((b) => !near(shadow, b) && !near(lit, b) && !inShadowOf(lit, b) && !blocks(b));
}

/** Ground-ring sample angles: RING_STEPS per circle, at radii 0.35h and 0.5h. */
export const RING_STEPS = 48;

/**
 * Ground points around a building: for each radius (0.35h, 0.5h), RING_STEPS
 * points at angle i·(360/RING_STEPS)° measured from +x toward +z. Shadows fall
 * toward ≈157° (away from the moon at (0.7, 1, −0.3)).
 */
function groundRing(g: BuildingGeometry, s: (x: number, y: number, z: number) => [number, number]): RingSample[] {
  const out: RingSample[] = [];
  for (const f of [0.35, 0.5]) {
    for (let i = 0; i < RING_STEPS; i++) {
      const deg = (i * 360) / RING_STEPS;
      const a = (deg * Math.PI) / 180;
      const [x, y] = s(g.x + f * g.height * Math.cos(a), g.base, g.z + f * g.height * Math.sin(a));
      out.push({ deg, x, y });
    }
  }
  return out;
}

export interface RingSample {
  deg: number;
  x: number;
  y: number;
}

/** Camera position and target that frame a building's +x and +z faces. */
export function poseFor(g: BuildingGeometry): { position: THREE.Vector3; target: THREE.Vector3 } {
  const d = Math.max(14, 0.9 * g.height);
  return {
    position: new THREE.Vector3(g.x + d, g.base + 0.6 * g.height + 6, g.z + d),
    target: new THREE.Vector3(g.x, g.base + g.height / 2, g.z),
  };
}
