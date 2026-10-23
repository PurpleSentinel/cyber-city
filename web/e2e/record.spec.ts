// SPDX-License-Identifier: MIT
// docs/TASKS.md T5.3: records the README clip from the demo replay. The video
// lands in e2e/artifacts/demo.webm with demo.json noting when the scan fired;
// `make media` cuts docs/media/demo.gif around it.
import { expect, test } from "@playwright/test";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { ARTIFACTS, authURL, waitReady } from "./util";

test.use({ video: { mode: "on", size: { width: 1280, height: 720 } } });

test("record the demo city for the README", async ({ page }) => {
  const start = Date.now();
  await page.goto(authURL("/?debug=1&quality=high"));
  await waitReady(page);
  await expect.poll(() => page.evaluate(() => window.__cybercity!.stats().buildings), { timeout: 30_000 }).toBeGreaterThanOrEqual(30);

  // Move in on the city, then orbit slowly until the demo's port scan fires
  // (15 s in at 4x) and for 6 s more.
  const cx = 640;
  const cy = 360;
  await page.mouse.move(cx, cy);
  for (let i = 0; i < 6; i++) {
    await page.mouse.wheel(0, -240);
    await page.waitForTimeout(150);
  }
  await page.mouse.down();
  let x = cx;
  let scanAt = 0;
  for (let i = 0; i < 400; i++) {
    x -= 1;
    await page.mouse.move(x, cy);
    await page.waitForTimeout(100);
    if (!scanAt && (await page.evaluate(() => window.__cybercity!.lastEvents().some((e) => e.kind === "port_scan")))) {
      scanAt = (Date.now() - start) / 1000;
    }
    if (scanAt && (Date.now() - start) / 1000 - scanAt >= 6) break;
  }
  await page.mouse.up();
  expect(scanAt).toBeGreaterThan(0);

  const video = page.video();
  await page.close();
  await video?.saveAs(join(ARTIFACTS, "demo.webm"));
  writeFileSync(join(ARTIFACTS, "demo.json"), `${JSON.stringify({ scan_s: Math.round(scanAt * 10) / 10 })}\n`);
});
