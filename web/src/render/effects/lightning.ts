// SPDX-License-Identifier: MIT
// Lightning strikes (docs/VISUAL_SPEC.md "Status effects"): a jagged additive
// line from y = 200 down to a roof, three segments re-randomised every 50 ms,
// lasting 0.4 s.
import * as THREE from "three";
import { STATUS } from "../../config";

const TOP_Y = 200;
const JITTER_S = 0.05;
const POINTS = 4; // three segments

interface Bolt {
  line: THREE.Line;
  age: number;
  sinceJitter: number;
  x: number;
  z: number;
  roof: number;
}

export class Lightning {
  readonly group = new THREE.Group();
  private readonly bolts: Bolt[] = [];
  private readonly material = new THREE.LineBasicMaterial({
    color: STATUS.lightningColor,
    transparent: true,
    blending: THREE.AdditiveBlending,
    depthWrite: false,
    toneMapped: false,
  });

  constructor(private readonly random: () => number = Math.random) {
    this.group.name = "lightning";
  }

  get active(): number {
    return this.bolts.length;
  }

  strike(x: number, z: number, roof: number): void {
    const geo = new THREE.BufferGeometry();
    geo.setAttribute("position", new THREE.BufferAttribute(new Float32Array(POINTS * 3), 3));
    const line = new THREE.Line(geo, this.material);
    line.frustumCulled = false;
    const bolt = { line, age: 0, sinceJitter: JITTER_S, x, z, roof };
    this.jitter(bolt);
    this.bolts.push(bolt);
    this.group.add(line);
  }

  frame(dt: number): void {
    for (let i = this.bolts.length - 1; i >= 0; i--) {
      const b = this.bolts[i] as Bolt;
      b.age += dt;
      if (b.age >= STATUS.lightningS) {
        this.group.remove(b.line);
        b.line.geometry.dispose();
        this.bolts.splice(i, 1);
        continue;
      }
      b.sinceJitter += dt;
      if (b.sinceJitter >= JITTER_S) this.jitter(b);
    }
  }

  private jitter(b: Bolt): void {
    b.sinceJitter = 0;
    const pos = b.line.geometry.getAttribute("position") as THREE.BufferAttribute;
    for (let p = 0; p < POINTS; p++) {
      const t = p / (POINTS - 1);
      const spread = p === 0 || p === POINTS - 1 ? 0 : 6;
      pos.setXYZ(
        p,
        b.x + (this.random() - 0.5) * 2 * spread,
        TOP_Y + (b.roof - TOP_Y) * t,
        b.z + (this.random() - 0.5) * 2 * spread,
      );
    }
    pos.needsUpdate = true;
  }
}
