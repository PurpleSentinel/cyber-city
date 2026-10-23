// Helpers for the e2e suite: login URL, debug-hook access, PNG pixel stats.
import { mkdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { PNG } from "pngjs";
import { STATE } from "./global-setup";

export const ARTIFACTS = join(import.meta.dirname, "artifacts");
mkdirSync(ARTIFACTS, { recursive: true });

/** The printed auth URL, redirecting to path (e.g. "/?debug=1"). */
export function authURL(path = "/?debug=1"): string {
  const url = readFileSync(join(STATE, "url.txt"), "utf8").trim();
  return `${url}&next=${encodeURIComponent(path)}`;
}

export function baseURL(): string {
  return readFileSync(join(STATE, "url.txt"), "utf8").trim().replace(/auth\?token=.*$/, "");
}

export type Point = [number, number];

export interface DebugStats {
  buildings: number;
  highways: number;
  particles: number;
  ticks: number;
  renderer: string;
  software: boolean;
  quality: "high" | "low";
  lit: boolean;
  wireframe: boolean;
  shadows: boolean;
}

declare global {
  interface Window {
    __cybercity?: {
      ready: boolean;
      stats(): DebugStats;
      hostIds(): string[];
      lastEvents(): { id: string; kind: string }[];
      probe(id: string): {
        px: Point[];
        pz: Point[];
        shadow: Point;
        lit: Point;
        sameSurface: boolean;
        ring: { deg: number; x: number; y: number }[];
      } | null;
      poseHost(): string | null;
    };
  }
}

/** Collects console errors and CSP violations from the start of a page's life. */
export async function watchProblems(page: Page): Promise<string[]> {
  const problems: string[] = [];
  page.on("pageerror", (e) => problems.push(`pageerror: ${e.message}`));
  page.on("console", (m) => {
    if (m.type() === "error") problems.push(m.text());
  });
  await page.addInitScript(() => {
    document.addEventListener("securitypolicyviolation", (e) => console.error(`CSP violation: ${e.violatedDirective} ${e.blockedURI}`));
  });
  return problems;
}

export async function waitReady(page: Page): Promise<void> {
  await page.waitForFunction(() => window.__cybercity?.ready === true, null, { timeout: 30_000 });
}

/** sRGB relative luminance of pixel (x, y), 0..1. */
export function luminance(png: PNG, x: number, y: number): number {
  const i = (png.width * Math.round(y) + Math.round(x)) * 4;
  const lin = (c: number) => {
    const s = (png.data[i + c] ?? 0) / 255;
    return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * lin(0) + 0.7152 * lin(1) + 0.0722 * lin(2);
}

/** Polygon shrunk toward its centroid by fraction f. */
export function shrink(poly: Point[], f: number): Point[] {
  const cx = poly.reduce((a, p) => a + p[0], 0) / poly.length;
  const cy = poly.reduce((a, p) => a + p[1], 0) / poly.length;
  return poly.map(([x, y]) => [cx + (x - cx) * (1 - f), cy + (y - cy) * (1 - f)]);
}

function inside(poly: Point[], x: number, y: number): boolean {
  let hit = false;
  for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
    const [xi, yi] = poly[i]!;
    const [xj, yj] = poly[j]!;
    if (yi > y !== yj > y && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi) hit = !hit;
  }
  return hit;
}

/** Luminance samples of every pixel inside a polygon. */
export function polygonLuminance(png: PNG, poly: Point[]): number[] {
  const xs = poly.map((p) => p[0]);
  const ys = poly.map((p) => p[1]);
  const out: number[] = [];
  for (let y = Math.max(0, Math.floor(Math.min(...ys))); y <= Math.min(png.height - 1, Math.ceil(Math.max(...ys))); y++) {
    for (let x = Math.max(0, Math.floor(Math.min(...xs))); x <= Math.min(png.width - 1, Math.ceil(Math.max(...xs))); x++) {
      if (inside(poly, x + 0.5, y + 0.5)) out.push(luminance(png, x, y));
    }
  }
  return out;
}

/** Mean luminance of a size×size patch centred on p, skipping pixels in exclude. */
export function patchLuminance(png: PNG, p: Point, size = 20, exclude: Point[][] = []): number {
  let sum = 0;
  let n = 0;
  for (let y = p[1] - size / 2; y < p[1] + size / 2; y++) {
    for (let x = p[0] - size / 2; x < p[0] + size / 2; x++) {
      if (x < 0 || y < 0 || x >= png.width || y >= png.height) continue;
      if (exclude.some((poly) => inside(poly, x + 0.5, y + 0.5))) continue;
      sum += luminance(png, x, y);
      n++;
    }
  }
  return n === 0 ? Number.NaN : sum / n; // NaN: every pixel excluded
}

export const mean = (xs: number[]) => (xs.length === 0 ? 0 : xs.reduce((a, b) => a + b, 0) / xs.length);
export const decodePNG = (buf: Buffer) => PNG.sync.read(buf);
