// SPDX-License-Identifier: MIT
// Rate and height model (docs/VISUAL_SPEC.md "Height"). Pure functions: no
// three.js, so Vitest can check the formulas directly.
import { RATE } from "../config";
import type { Flow } from "../protocol";

/** Per-host traffic rate in bytes/s, an EWMA in packet time. */
export interface RateState {
  rate: number;
  /** Packet time (ms) of the last sample; 0 before the first. */
  lastTs: number;
}

export const newRate = (): RateState => ({ rate: 0, lastTs: 0 });

const halfLifeFactor = (dtS: number): number => 2 ** (-dtS / RATE.halfLifeS);

/**
 * Folds one tick's bytes into the EWMA:
 * sample = bytes / window, alpha = 1 − 2^(−Δt / half-life).
 * Returns the sample (bytes/s) so callers can detect spikes.
 */
export function addSample(s: RateState, bytes: number, windowMs: number, tickTs: number): number {
  const sample = bytes / (windowMs / 1000);
  const dtS = s.lastTs === 0 ? Infinity : Math.max(0, (tickTs - s.lastTs) / 1000);
  const alpha = 1 - halfLifeFactor(dtS);
  s.rate += alpha * (sample - s.rate);
  s.lastTs = tickTs;
  return sample;
}

/** The rate decayed to packet time nowTs, so quiet hosts sink between samples. */
export function decayedRate(s: RateState, nowTs: number): number {
  if (s.lastTs === 0) return 0;
  return s.rate * halfLifeFactor(Math.max(0, (nowTs - s.lastTs) / 1000));
}

/** Target building height for a rate: min(40, 1 + 6·log10(1 + rate/1000)). */
export function targetHeight(rateBps: number): number {
  return Math.min(RATE.maxHeight, 1 + RATE.heightScale * Math.log10(1 + Math.max(0, rateBps) / RATE.heightDivisor));
}

/** Moves a displayed height toward its target with time constant τ = 0.25 s. */
export function stepHeight(h: number, target: number, dtS: number): number {
  return h + (target - h) * (1 - Math.exp(-Math.max(0, dtS) / RATE.displayTauS));
}

/** Bytes per host id in a tick: each flow counts for both endpoints. */
export function bytesByHost(flows: readonly Flow[]): Map<string, number> {
  const out = new Map<string, number>();
  for (const f of flows) {
    out.set(f.src, (out.get(f.src) ?? 0) + f.bytes);
    if (f.dst !== f.src) out.set(f.dst, (out.get(f.dst) ?? 0) + f.bytes);
  }
  return out;
}
