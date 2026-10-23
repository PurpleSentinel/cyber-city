import { NET } from "../config";

/**
 * Reconnect delay in milliseconds for the given attempt (0-based):
 * 0.5 s × 2ⁿ capped at 10 s, with ±20% jitter (docs/VISUAL_SPEC.md "Reconnect").
 * `random` returns [0, 1) and is injectable for tests.
 */
export function backoffDelayMs(attempt: number, random: () => number = Math.random): number {
  const base = Math.min(NET.backoffMaxS, NET.backoffBaseS * 2 ** Math.max(0, attempt));
  const jitter = 1 + (random() * 2 - 1) * NET.backoffJitter;
  return Math.round(base * jitter * 1000);
}
