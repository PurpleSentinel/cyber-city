// docs/TESTING.md "End-to-end" step 7: without the token the page holds no data.
import { expect, test } from "@playwright/test";
import { baseURL, watchProblems } from "./util";

test("opening the page without the token shows Invalid link", async ({ page }) => {
  const problems = await watchProblems(page);
  await page.goto(baseURL());
  await expect(page.locator("#status")).toContainText("Invalid", { timeout: 15_000 });
  // The only expected error is the 401 from /api/session.
  expect(problems.filter((p) => !p.includes("401") && !p.includes("GPU stall"))).toEqual([]);
});
