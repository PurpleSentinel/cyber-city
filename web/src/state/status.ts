// Status-effect triggers (docs/VISUAL_SPEC.md "Status effects"). Pure.
import { STATUS } from "../config";

/**
 * How degraded a host looks, 0..1:
 * clamp(max((rtt − 150)/350, (retrans − 0.02)/0.08), 0, 1).
 */
export function degradedLevel(rttMs: number | null, retransRate: number): number {
  const byRtt = rttMs === null ? 0 : (rttMs - STATUS.degradedRttMs) / STATUS.degradedRttSpanMs;
  const byRetrans = (retransRate - STATUS.degradedRetrans) / STATUS.degradedRetransSpan;
  return Math.min(1, Math.max(0, byRtt, byRetrans));
}

export type SampleEffect = "none" | "flash" | "lightning";

/**
 * Effect for a new traffic sample against the host's rate before it:
 * lightning above 10× and 10 MB/s, a spike flash above 4× and 1 MB/s.
 */
export function sampleEffect(prevRateBps: number, sampleBps: number): SampleEffect {
  if (sampleBps > STATUS.lightningFactor * prevRateBps && sampleBps > STATUS.lightningMinBps) return "lightning";
  if (sampleBps > STATUS.spikeFactor * prevRateBps && sampleBps > STATUS.spikeMinBps) return "flash";
  return "none";
}
