// SPDX-License-Identifier: MIT
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import type { Hello, Tick } from "../src/protocol";
import { rendererInfo } from "../src/render/rendererInfo";
import { Store } from "../src/state/store";

const read = <T>(name: string): T =>
  JSON.parse(readFileSync(new URL(`../../testdata/protocol/${name}`, import.meta.url), "utf8")) as T;

describe("Store", () => {
  it("rebuilds state from each hello", () => {
    const s = new Store();
    const resets: number[] = [];
    s.subscribe({ reset: () => resets.push(1) });
    const hello = read<Hello>("hello.json");
    s.applyHello(hello);
    expect(s.hosts.size).toBe(2);
    expect([...s.districts.keys()].sort()).toEqual(["core", "corp", "outlands"]);
    s.applyTick(read<Tick>("tick.json"));
    // A new hello replaces everything (reconnect).
    s.applyHello({ ...hello, hosts: [], districts: [] });
    expect(s.hosts.size).toBe(0);
    expect(s.districts.size).toBe(0);
    expect(resets).toHaveLength(2);
  });

  it("applies ticks: upserts, removals and events", () => {
    const s = new Store();
    s.applyHello(read<Hello>("hello.json"));
    const added: string[] = [];
    const updated: string[] = [];
    const removed: [string, boolean][] = [];
    s.subscribe({
      hostAdded: (h) => added.push(h.id),
      hostUpdated: (h) => updated.push(h.id),
      hostRemoved: (id, viaEvent) => removed.push([id, viaEvent]),
    });
    s.applyTick(read<Tick>("tick.json"));
    expect(updated).toEqual(["10.0.1.10"]);
    expect(s.lastFlows).toHaveLength(2);

    const events = read<Tick>("events.json");
    s.applyHello({ ...read<Hello>("hello.json"), hosts: [{ ...read<Hello>("hello.json").hosts[0]!, id: "10.0.1.30" }] });
    s.applyTick(events);
    expect(removed).toEqual([["10.0.1.30", true]]); // removed with its host_idle event
    expect(s.events.map((e) => e.kind)).toEqual(["host_idle", "flood", "port_scan"]);
  });
});

describe("rendererInfo", () => {
  const gl = (name: string | null, ext = true) => ({
    RENDERER: 0x1f01,
    getExtension: () => (ext ? { UNMASKED_RENDERER_WEBGL: 0x9246 } : null),
    getParameter: () => name,
  });
  it("detects software renderers", () => {
    expect(rendererInfo(gl("ANGLE (Google, Vulkan 1.3.0 (SwiftShader Device (Subzero)))"))).toEqual({
      name: "ANGLE (Google, Vulkan 1.3.0 (SwiftShader Device (Subzero)))",
      software: true,
    });
    expect(rendererInfo(gl("llvmpipe (LLVM 17.0.6, 256 bits)")).software).toBe(true);
    expect(rendererInfo(gl("Mesa Intel(R) Graphics (ADL GT2)")).software).toBe(false);
    expect(rendererInfo(gl(null, false))).toEqual({ name: "unknown", software: false });
  });
});
