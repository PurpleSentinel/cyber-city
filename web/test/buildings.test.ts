import { readFileSync } from "node:fs";
import * as THREE from "three";
import { describe, expect, it } from "vitest";
import type { Hello, Host } from "../src/protocol";
import { Buildings, servicesMask } from "../src/render/buildings";

const hello = JSON.parse(readFileSync(new URL("../../testdata/protocol/hello.json", import.meta.url), "utf8")) as Hello;
const host = (id: string, extra: Partial<Host> = {}): Host => ({ ...hello.hosts[0]!, id, ...extra });

describe("Buildings", () => {
  it("passes the shading guards (D17)", () => {
    const b = new Buildings(10);
    const mat = b.mesh.material as THREE.MeshStandardMaterial;
    expect(mat).toBeInstanceOf(THREE.MeshStandardMaterial);
    expect(mat.wireframe).toBe(false);
    expect(b.mesh.castShadow).toBe(true);
    expect(b.mesh.receiveShadow).toBe(true);
    expect(b.mesh.customDepthMaterial).toBeInstanceOf(THREE.MeshDepthMaterial);
  });

  it("keeps one instance per host and frees slots after the fade-out", () => {
    const b = new Buildings(10);
    for (const id of ["a", "b", "c"]) b.add(host(id), { x: 0, z: 0 }, true);
    expect(b.size).toBe(3);
    expect(b.mesh.count).toBe(3);
    b.remove("b");
    expect(b.frame(0.5)).toEqual([]); // still fading
    expect(b.frame(0.6)).toEqual(["b"]); // fade-out complete (1.0 s)
    expect(b.size).toBe(2);
    b.add(host("d"), { x: 4, z: 4 }, true);
    expect(b.mesh.count).toBe(3); // reused the freed instance
  });

  it("grows a building from the ground and toward its traffic height", () => {
    const b = new Buildings(10);
    b.add(host("a"), { x: 8, z: -4 }, true);
    b.applyBytes(new Map([["a", 100_000]]), 100, 1_000); // 1 MB/s → target ≈ 19
    for (let i = 0; i < 60; i++) b.frame(1 / 60);
    const g = b.geometry("a")!;
    expect(g.height).toBeGreaterThan(15);
    expect(g.base).toBeCloseTo(0.2);
    const m = new THREE.Matrix4();
    b.mesh.getMatrixAt(0, m);
    const pos = new THREE.Vector3();
    const scale = new THREE.Vector3();
    m.decompose(pos, new THREE.Quaternion(), scale);
    expect([pos.x, pos.z]).toEqual([8, -4]);
    expect(scale.y).toBeCloseTo(g.height, 5);
  });

  it("packs services into the category bitmask", () => {
    const h = host("a", {
      services: [
        { port: 443, proto: "tcp", category: "https", confirmed: true },
        { port: 22, proto: "tcp", category: "ssh", confirmed: true },
        { port: 9, proto: "tcp", category: "mystery", confirmed: false },
      ],
    });
    expect(servicesMask(h)).toBe((1 << 0) | (1 << 3) | (1 << 7)); // https, ssh, other
    expect(servicesMask(host("b", { services: [] }))).toBe(0);
  });
});
