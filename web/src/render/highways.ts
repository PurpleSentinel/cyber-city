// Highway rendering (docs/VISUAL_SPEC.md "Highways"): instanced flat quads,
// additive, one or two segments per road.
import * as THREE from "three";
import { CATEGORY_COLORS, HIGHWAYS, MATERIALS } from "../config";
import type { Point } from "../layout/layout";
import type { Road } from "../state/highways";
import { route } from "../state/highways";
import { glowAttributes, glowMaterial } from "./glow";

/** Roads float 0.1 above the plot tops (VISUAL_SPEC "Highways"). */
export const ROAD_Y = MATERIALS.plot.height + HIGHWAYS.y;


export interface EndPoint {
  point: Point;
  local: boolean;
}

export class Highways {
  readonly mesh: THREE.InstancedMesh;
  private readonly color: THREE.InstancedBufferAttribute;
  private readonly alpha: THREE.InstancedBufferAttribute;
  private readonly dummy = new THREE.Object3D();
  private readonly tmp = new THREE.Color();
  roadCount = 0;

  constructor(capacity = HIGHWAYS.max * 2) {
    const geo = new THREE.PlaneGeometry(1, 1);
    geo.rotateX(-Math.PI / 2); // lie flat in XZ
    ({ color: this.color, alpha: this.alpha } = glowAttributes(geo, capacity));
    this.mesh = new THREE.InstancedMesh(geo, glowMaterial(), capacity);
    this.mesh.count = 0;
    this.mesh.frustumCulled = false;
    this.mesh.name = "highways";
  }

  /** Draws roads between the endpoints found by lookup (unknown ends are skipped). */
  update(roads: readonly Road[], lookup: (id: string) => EndPoint | null): void {
    let n = 0;
    this.roadCount = 0;
    for (const r of roads) {
      const a = lookup(r.a);
      const b = lookup(r.b);
      if (!a || !b) continue;
      this.roadCount++;
      this.tmp.setHex(CATEGORY_COLORS[r.category] ?? CATEGORY_COLORS.other ?? 0x8a8fa3);
      for (const s of route(a.point, b.point, a.local && b.local)) {
        if (n >= this.mesh.instanceMatrix.count) break;
        const dx = s.to.x - s.from.x;
        const dz = s.to.z - s.from.z;
        this.dummy.position.set((s.from.x + s.to.x) / 2, ROAD_Y, (s.from.z + s.to.z) / 2);
        this.dummy.rotation.set(0, -Math.atan2(dz, dx), 0);
        this.dummy.scale.set(Math.hypot(dx, dz) + r.width, 1, r.width);
        this.dummy.updateMatrix();
        this.mesh.setMatrixAt(n, this.dummy.matrix);
        this.color.setXYZ(n, this.tmp.r, this.tmp.g, this.tmp.b);
        this.alpha.setX(n, r.opacity);
        n++;
      }
    }
    this.mesh.count = n;
    this.mesh.instanceMatrix.needsUpdate = true;
    this.color.needsUpdate = true;
    this.alpha.needsUpdate = true;
  }
}
