// docs/TESTING.md "End-to-end" step 5: the city is solid, lit and shadowed,
// never wireframe (D17). Measured from pixels at a fixed camera pose.
import { expect, test } from "@playwright/test";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { ARTIFACTS, authURL, decodePNG, mean, patchLuminance, polygonLuminance, shrink, waitReady } from "./util";

const BG_LUMINANCE = 0.0015; // #05050a

test("buildings are filled, lit and cast shadows", async ({ page }) => {
  await page.goto(authURL("/?debug=1&quality=high&pose=auto&hide=traffic"));
  await waitReady(page);
  await page.waitForFunction(() => window.__cybercity!.poseHost() !== null, null, { timeout: 45_000 });
  await page.waitForTimeout(2_000); // camera and height settle

  const stats = await page.evaluate(() => window.__cybercity!.stats());
  expect(stats).toMatchObject({ wireframe: false, lit: true, shadows: true });

  const id = await page.evaluate(() => window.__cybercity!.poseHost()!);
  const probe = await page.evaluate((h) => window.__cybercity!.probe(h)!, id);
  expect(probe.sameSurface).toBe(true);

  const shot = await page.screenshot();
  writeFileSync(join(ARTIFACTS, "shading.png"), shot);
  writeFileSync(join(ARTIFACTS, "shading-probe.json"), JSON.stringify({ id, probe }));
  const png = decodePNG(shot);

  // Filled: ≥ 90% of pixels inside each face differ from the background.
  for (const face of [probe.px, probe.pz]) {
    const lum = polygonLuminance(png, shrink(face, 0.15));
    expect(lum.length).toBeGreaterThan(50);
    const filled = lum.filter((l) => Math.abs(l - BG_LUMINANCE) > 0.02 * Math.max(l, BG_LUMINANCE) && l > BG_LUMINANCE * 1.5);
    expect(filled.length / lum.length).toBeGreaterThanOrEqual(0.9);
  }

  // Lit: the moon-lit +x facade body is brighter than the sky-lit +z body by
  // ≥ 8%. The 30th percentile excludes the emissive windows, which are the
  // same on both faces and would otherwise dominate a mean.
  const pxAll = polygonLuminance(png, shrink(probe.px, 0.15));
  const pzAll = polygonLuminance(png, shrink(probe.pz, 0.15));
  const px = mean(pxAll);
  const pz = mean(pzAll);
  const pct = (xs: number[], q: number) => [...xs].sort((a, b) => a - b)[Math.floor(q * (xs.length - 1))] ?? 0;
  const diag = `px=${px.toFixed(4)} pz=${pz.toFixed(4)} p30 px=${pct(pxAll, 0.3).toFixed(4)} pz=${pct(pzAll, 0.3).toFixed(4)}`;
  test.info().annotations.push({ type: "luminance", description: diag });
  console.log(`shading: ${diag}`);
  const bodyX = pct(pxAll, 0.3);
  const bodyZ = pct(pzAll, 0.3);
  expect(bodyX - bodyZ).toBeGreaterThanOrEqual(0.08 * bodyX);

  // Shadowed: around the building, the darkest ground in the sector away from
  // the moon (112°–202°, camera-visible) is ≥ 5% darker than the median of the
  // moon-facing, camera-visible ground (300°–90°). The cast shadow is a narrow
  // wedge, so the ring samples it at 7.5° steps on two radii.
  const faces = [probe.px, probe.pz];
  const sample = (r: { x: number; y: number }) => patchLuminance(png, [r.x, r.y], 5, faces);
  const shadowSector = probe.ring.filter((r) => r.deg >= 112 && r.deg <= 202).map(sample).filter((v) => !Number.isNaN(v));
  const litSector = probe.ring
    .filter((r) => r.deg >= 300 || r.deg <= 90)
    .map(sample)
    .filter((v) => !Number.isNaN(v))
    .sort((a, b) => a - b);
  expect(shadowSector.length).toBeGreaterThan(10);
  expect(litSector.length).toBeGreaterThan(10);
  const shadow = Math.min(...shadowSector);
  const lit = litSector[Math.floor(litSector.length / 2)] ?? 0;
  test.info().annotations.push({ type: "ground", description: `shadow=${shadow.toFixed(4)} lit=${lit.toFixed(4)}` });
  console.log(`ground: ${id} shadow=${shadow.toFixed(4)} lit=${lit.toFixed(4)}`);
  expect(shadow).toBeLessThanOrEqual(0.95 * lit);
});
