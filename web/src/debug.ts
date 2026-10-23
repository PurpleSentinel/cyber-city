// Read-only test hook (docs/VISUAL_SPEC.md "Debug hook"), installed only with
// ?debug=1. It never offers a way to change state.

export interface DebugStats {
  buildings: number;
  highways: number;
  particles: number;
  ticks: number;
  events: number;
  renderer: string;
  software: boolean;
  quality: "high" | "low";
  lit: boolean;
  wireframe: boolean;
  shadows: boolean;
}

export interface ProbeResult {
  px: [number, number][];
  pz: [number, number][];
  shadow: [number, number];
  lit: [number, number];
  sameSurface: boolean;
  /** Ground points around the building (degrees from +x toward +z, screen px). */
  ring: { deg: number; x: number; y: number }[];
}

export interface DebugSources {
  stats(): DebugStats;
  hostIds(): string[];
  lastEvents(): { id: string; kind: string }[];
  probe(hostId: string): ProbeResult | null;
  poseHost(): string | null;
}

export interface DebugHandle {
  markReady(): void;
}

export function debugEnabled(search: string = window.location.search): boolean {
  return new URLSearchParams(search).get("debug") === "1";
}

export function installDebug(sources: DebugSources, target: object = window): DebugHandle {
  let ready = false;
  const api = Object.freeze({
    get ready() {
      return ready;
    },
    stats: () => sources.stats(),
    hostIds: () => sources.hostIds(),
    lastEvents: () => sources.lastEvents(),
    probe: (id: string) => sources.probe(id),
    poseHost: () => sources.poseHost(),
  });
  Object.defineProperty(target, "__cybercity", { value: api, writable: false, configurable: false });
  return { markReady: () => (ready = true) };
}
