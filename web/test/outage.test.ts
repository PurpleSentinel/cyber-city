import { readFileSync } from "node:fs";
import * as THREE from "three";
import { describe, expect, it } from "vitest";
import type { Hello, Host } from "../src/protocol";
import { Buildings } from "../src/render/buildings";
import { Outage, dissolveAt } from "../src/render/effects/outage";

const hello = JSON.parse(readFileSync(new URL("../../testdata/protocol/hello.json", import.meta.url), "utf8")) as Hello;
const host = (id: string): Host => ({ ...hello.hosts[0]!, id });

function scaleYOf(b: Buildings, index: number): number {
  const m = new THREE.Matrix4();
  b.mesh.getMatrixAt(index, m);
  return new THREE.Vector3().setFromMatrixScale(m).y;
}

describe("Outage", () => {
  it("flickers for 2 s, then dissolves and sinks over 3 s", () => {
    expect(dissolveAt(0)).toEqual({ dissolve: 0, scaleY: 1 });
    expect(dissolveAt(2)).toEqual({ dissolve: 0, scaleY: 1 });
    expect(dissolveAt(3.5).dissolve).toBeCloseTo(0.5);
    expect(dissolveAt(3.5).scaleY).toBeCloseTo(0.5);
    expect(dissolveAt(5)).toEqual({ dissolve: 1, scaleY: 0 });

    let n = 0;
    const o = new Outage(() => (n++ % 2 === 0 ? 0.2 : 0.8)); // off, on, off, …
    const seen = new Set<boolean>();
    for (let i = 0; i < 19; i++) seen.add(o.step(0.1).windowsOff); // 10 Hz flicker up to 1.9 s
    expect(seen).toEqual(new Set([true, false]));
    const dissolving = o.step(1.6); // 3.5 s
    expect(dissolving.windowsOff).toBe(false);
    expect(dissolving.dissolve).toBeCloseTo(0.5);
    expect(dissolving.done).toBe(false);
    expect(o.step(1.5).done).toBe(true); // 5 s
  });
});

describe("Buildings outage", () => {
  it("drives aState.z and scale y, then returns the instance to the free list", () => {
    const b = new Buildings(10);
    b.add(host("idle"), { x: 0, z: 0 }, true);
    b.add(host("busy"), { x: 4, z: 0 }, true);
    b.frame(1); // fully faded in
    const before = scaleYOf(b, 0);
    const state = b.mesh.geometry.getAttribute("aState") as THREE.InstancedBufferAttribute;

    b.outage("idle", () => 0.2); // windows off on every toggle
    b.frame(0.1);
    expect(state.getZ(0)).toBe(-1);
    expect(b.idAt(0)).toBeNull(); // not pickable while going dark
    b.frame(1.9); // t = 2.0: dissolve starts
    b.frame(1.5); // t = 3.5
    expect(state.getZ(0)).toBeCloseTo(0.5);
    expect(scaleYOf(b, 0)).toBeCloseTo(before * 0.5, 1);
    expect(b.freeSlots).toBe(0);
    expect(b.frame(1.5)).toEqual(["idle"]); // t = 5
    expect(b.freeSlots).toBe(1);
    expect(b.has("idle")).toBe(false);
    b.add(host("new"), { x: 8, z: 0 }, true);
    expect(b.mesh.count).toBe(2); // the freed instance is reused
    expect(state.getZ(0)).toBe(0);
  });

  it("keeps a host that returns during its outage", () => {
    const b = new Buildings(10);
    b.add(host("h"), { x: 0, z: 0 }, true);
    b.outage("h");
    b.frame(1);
    b.add(host("h"), { x: 0, z: 0 }, true);
    expect(b.frame(5)).toEqual([]);
    expect(b.has("h")).toBe(true);
    expect(b.idAt(0)).toBe("h");
  });
});
