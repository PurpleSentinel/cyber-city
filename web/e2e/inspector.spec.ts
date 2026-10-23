// SPDX-License-Identifier: MIT
// T3.10: clicking a building opens the inspector for that host.
import { expect, test } from "@playwright/test";
import { authURL, waitReady } from "./util";

test("clicking a building shows its details", async ({ page }) => {
  await page.goto(authURL("/?debug=1&quality=high&pose=auto"));
  await waitReady(page);
  await page.waitForFunction(() => window.__cybercity!.poseHost() !== null, null, { timeout: 45_000 });
  await page.waitForTimeout(1_500);
  const { id, x, y } = await page.evaluate(() => {
    const d = window.__cybercity!;
    const h = d.poseHost()!;
    const px = d.probe(h)!.px;
    return { id: h, x: px.reduce((a, p) => a + p[0], 0) / px.length, y: px.reduce((a, p) => a + p[1], 0) / px.length };
  });
  await page.mouse.move(x, y);
  await expect(page.locator("#tooltip")).toContainText(id);
  await page.mouse.click(x, y);
  await expect(page.locator("#inspector")).toHaveAttribute("data-host", id);
  await expect(page.locator("#inspector")).toContainText("(heuristic)");
  await page.keyboard.press("Escape");
  await expect(page.locator("#inspector")).toBeHidden();
});
