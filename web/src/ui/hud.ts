// SPDX-License-Identifier: MIT
// HUD (docs/VISUAL_SPEC.md "HUD and interaction"). Formatting and averaging
// are pure; the Hud class only writes text into existing elements.
import type { Stats } from "../protocol";

const UNITS = ["B", "kB", "MB", "GB", "TB"];

/** Human-readable bytes per second, e.g. "1.2 MB/s". */
export function formatRate(bytesPerS: number): string {
  let v = Math.max(0, bytesPerS);
  let i = 0;
  while (v >= 1000 && i < UNITS.length - 1) {
    v /= 1000;
    i++;
  }
  return `${v < 10 && i > 0 ? v.toFixed(1) : Math.round(v)} ${UNITS[i]}/s`;
}

/** Packets per second, e.g. "8.1k pkt/s". */
export function formatPps(pps: number): string {
  if (pps >= 1e6) return `${(pps / 1e6).toFixed(1)}M pkt/s`;
  if (pps >= 1e3) return `${(pps / 1e3).toFixed(1)}k pkt/s`;
  return `${Math.round(pps)} pkt/s`;
}

/** "shown / total (+hidden)" host count. */
export function formatHosts(shown: number, total: number, hidden: number): string {
  return `Hosts ${shown} / ${total}${hidden > 0 ? ` (+${hidden} hidden)` : ""}`;
}

/** 1 s moving average of packets and bytes per second over tick stats (packet time). */
export class MovingRate {
  private samples: { ts: number; packets: number; bytes: number; windowMs: number }[] = [];

  add(ts: number, windowMs: number, s: Pick<Stats, "packets" | "bytes">): void {
    this.samples.push({ ts, packets: s.packets, bytes: s.bytes, windowMs });
    while (this.samples.length > 0 && (this.samples[0]?.ts ?? ts) <= ts - 1000) this.samples.shift();
  }

  /** Averages over the last second of packet time (empty windows count as zero). */
  rates(): { pps: number; bps: number } {
    let packets = 0;
    let bytes = 0;
    for (const s of this.samples) {
      packets += s.packets;
      bytes += s.bytes;
    }
    return { pps: packets, bps: bytes }; // totals over 1 s are already per-second
  }

  clear(): void {
    this.samples = [];
  }
}

export interface HudFields {
  status: string;
  source: string;
  traffic: string;
  hosts: string;
  drops: string;
  dropsAlert: boolean;
}

/** Writes HUD fields into the overlay elements (ids in index.html). */
export class Hud {
  private readonly el: Record<keyof HudFields, HTMLElement | null>;

  constructor(doc: Document = document) {
    this.el = {
      status: doc.getElementById("status"),
      source: doc.getElementById("source"),
      traffic: doc.getElementById("traffic"),
      hosts: doc.getElementById("hosts"),
      drops: doc.getElementById("drops"),
      dropsAlert: null,
    };
  }

  set(f: Partial<HudFields>): void {
    for (const k of ["status", "source", "traffic", "hosts", "drops"] as const) {
      const v = f[k];
      const node = this.el[k];
      if (v !== undefined && node && node.textContent !== v) node.textContent = v;
    }
    if (f.dropsAlert !== undefined) this.el.drops?.classList.toggle("alert", f.dropsAlert);
  }
}

export const HELP_TEXT = [
  "Space — pause / resume",
  "L — toggle low quality",
  "F — fly to the selected building",
  "Esc — clear selection",
  "? — this help",
  "Drag to orbit, scroll to zoom",
];
