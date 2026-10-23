// Searchlight for port_scan events (docs/VISUAL_SPEC.md "Event effects"): an
// additive cone from the scanner's roof that aims at each target in turn,
// while the targets carry an alert outline.
import * as THREE from "three";
import { EFFECTS, PALETTE } from "../../config";

const S = EFFECTS.searchlight;

export interface Footprint {
  x: number;
  z: number;
  base: number;
  height: number;
}

/** Index of the target aimed at t seconds in, or -1 once the effect is over. */
export function aimIndex(t: number, targets: number): number {
  if (targets === 0 || t < 0 || t >= S.durationS) return -1;
  return Math.floor(t / S.perTargetS) % targets;
}

interface Beam {
  src: string;
  targets: string[];
  age: number;
  cone: THREE.Mesh;
  outlines: Map<string, THREE.LineSegments>;
}

const DOWN = new THREE.Vector3(0, -1, 0);

export class Searchlights {
  readonly group = new THREE.Group();
  private readonly beams: Beam[] = [];
  private readonly coneGeometry = new THREE.ConeGeometry(S.radius, S.height, 24, 1, true);
  private readonly edges: THREE.EdgesGeometry;
  private readonly coneMaterial = new THREE.MeshBasicMaterial({
    color: PALETTE.alert,
    transparent: true,
    opacity: S.opacity,
    blending: THREE.AdditiveBlending,
    depthWrite: false,
    side: THREE.DoubleSide,
    toneMapped: false,
  });
  private readonly outlineMaterial = new THREE.LineBasicMaterial({ color: PALETTE.alert, toneMapped: false });
  private readonly dir = new THREE.Vector3();

  constructor(private readonly footprint: (id: string) => Footprint | null) {
    this.group.name = "searchlights";
    this.coneGeometry.translate(0, -S.height / 2, 0); // apex at the origin, opening down -y
    const box = new THREE.BoxGeometry(2, 1, 2);
    box.translate(0, 0.5, 0);
    this.edges = new THREE.EdgesGeometry(box);
    box.dispose();
  }

  get active(): number {
    return this.beams.length;
  }

  /** Starts a searchlight; false when the cap is reached or the scanner is not drawn. */
  start(src: string, targets: string[]): boolean {
    if (this.beams.length >= EFFECTS.maxSearchlights || !this.footprint(src)) return false;
    const cone = new THREE.Mesh(this.coneGeometry, this.coneMaterial);
    cone.frustumCulled = false;
    this.group.add(cone);
    const beam: Beam = { src, targets: targets.filter((t) => t !== src), age: 0, cone, outlines: new Map() };
    for (const id of beam.targets) {
      // Newest wins: an older searchlight gives up its outline on a shared target.
      for (const other of this.beams) this.dropOutline(other, id);
      const line = new THREE.LineSegments(this.edges, this.outlineMaterial);
      this.group.add(line);
      beam.outlines.set(id, line);
    }
    this.beams.push(beam);
    this.place(beam);
    return true;
  }

  /** The target the beam started for src is aiming at, if any (for tests). */
  aimedAt(src: string): string | null {
    const b = this.beams.find((x) => x.src === src);
    if (!b) return null;
    return b.targets[aimIndex(b.age, b.targets.length)] ?? null;
  }

  frame(dt: number): void {
    for (let i = this.beams.length - 1; i >= 0; i--) {
      const b = this.beams[i] as Beam;
      b.age += dt;
      if (b.age >= S.durationS || !this.footprint(b.src)) {
        this.group.remove(b.cone);
        for (const id of [...b.outlines.keys()]) this.dropOutline(b, id);
        this.beams.splice(i, 1);
        continue;
      }
      this.place(b);
    }
  }

  private dropOutline(b: Beam, id: string): void {
    const line = b.outlines.get(id);
    if (!line) return;
    this.group.remove(line);
    b.outlines.delete(id);
  }

  private place(b: Beam): void {
    const src = this.footprint(b.src);
    if (!src) return;
    b.cone.position.set(src.x, src.base + src.height, src.z);
    const target = this.footprint(b.targets[aimIndex(b.age, b.targets.length)] ?? "");
    if (target) {
      this.dir.set(target.x - src.x, target.base + target.height - (src.base + src.height), target.z - src.z);
      if (this.dir.lengthSq() > 1e-6) b.cone.quaternion.setFromUnitVectors(DOWN, this.dir.normalize());
    }
    for (const [id, line] of b.outlines) {
      const f = this.footprint(id);
      line.visible = f !== null;
      if (!f) continue;
      line.position.set(f.x, f.base, f.z);
      line.scale.set(1.08, Math.max(f.height, 0.1) * 1.02, 1.08);
    }
  }
}
