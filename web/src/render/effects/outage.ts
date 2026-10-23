// SPDX-License-Identifier: MIT
// Power outage for host_idle events (docs/VISUAL_SPEC.md "Event effects"):
// the windows flicker at random for 2 s, then the building dissolves over 3 s
// while it sinks; the instance is freed afterwards.
import { EFFECTS } from "../../config";

const O = EFFECTS.outage;

export interface OutageFrame {
  /** Windows dark this frame (flicker phase). */
  windowsOff: boolean;
  /** Dissolve threshold for aState.z, 0 → 1. */
  dissolve: number;
  /** Multiplier on the building's height, 1 → 0. */
  scaleY: number;
  done: boolean;
}

/** Dissolve progress and height factor t seconds into an outage. */
export function dissolveAt(t: number): { dissolve: number; scaleY: number } {
  const k = Math.min(1, Math.max(0, (t - O.flickerS) / O.dissolveS));
  return { dissolve: k, scaleY: 1 - k };
}

export class Outage {
  private t = 0;
  private sinceToggle = 0;
  private off = false;

  constructor(private readonly random: () => number = Math.random) {}

  get elapsed(): number {
    return this.t;
  }

  step(dt: number): OutageFrame {
    this.t += dt;
    this.sinceToggle += dt;
    if (this.t < O.flickerS && this.sinceToggle >= 1 / O.flickerHz) {
      this.sinceToggle = 0;
      this.off = this.random() < 0.5;
    }
    const flickering = this.t < O.flickerS;
    const { dissolve, scaleY } = dissolveAt(this.t);
    return {
      windowsOff: flickering && this.off,
      dissolve,
      scaleY,
      done: this.t >= O.flickerS + O.dissolveS,
    };
  }
}
