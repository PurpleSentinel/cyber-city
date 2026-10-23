import { describe, expect, it } from "vitest";
import type { DebugSources } from "../src/debug";
import { debugEnabled, installDebug } from "../src/debug";

const sources: DebugSources = {
  stats: () => ({
    buildings: 3, highways: 0, particles: 0, ticks: 7, events: 0, renderer: "x",
    software: false, quality: "high", lit: true, wireframe: false, shadows: true,
  }),
  hostIds: () => ["a", "b", "c"],
  lastEvents: () => [],
  probe: () => null,
  poseHost: () => null,
};

describe("debug hook", () => {
  it("is enabled only by ?debug=1", () => {
    expect(debugEnabled("?debug=1")).toBe(true);
    expect(debugEnabled("?debug=0")).toBe(false);
    expect(debugEnabled("")).toBe(false);
  });

  it("is read-only and reports readiness", () => {
    const target: Record<string, unknown> = {};
    const handle = installDebug(sources, target);
    const api = target.__cybercity as { ready: boolean; stats(): { buildings: number }; hostIds(): string[] };
    expect(api.ready).toBe(false);
    handle.markReady();
    expect(api.ready).toBe(true);
    expect(api.stats().buildings).toBe(3);
    expect(api.hostIds()).toEqual(["a", "b", "c"]);
    expect(() => {
      (target as { __cybercity: unknown }).__cybercity = {};
    }).toThrow();
    expect(() => {
      (api as unknown as { stats: unknown }).stats = null;
    }).toThrow();
  });
});
