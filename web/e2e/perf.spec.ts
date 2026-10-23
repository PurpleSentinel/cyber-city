// docs/TASKS.md T5.4: frame time with about 2000 hosts. Starts its own server
// on testdata/pcap/many_hosts.pcap and records the numbers in
// e2e/artifacts/perf.json for tools/perfcheck. It asserts only that the city
// loaded: regressions are judged against the stored baseline by the
// non-blocking perf CI job.
import { expect, test } from "@playwright/test";
import { type ChildProcess, spawn } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { ROOT } from "./global-setup";
import { ARTIFACTS, waitReady } from "./util";

const SAMPLE_MS = 5000;

let server: ChildProcess | undefined;
let url = "";
let dir = "";

test.beforeAll(async () => {
  dir = mkdtempSync(join(tmpdir(), "cybercity-perf-"));
  const urlFile = join(dir, "url.txt");
  server = spawn(
    join(ROOT, "bin", "cybercity"),
    ["--read", join(ROOT, "testdata", "pcap", "many_hosts.pcap"), "--replay-speed", "1", "--loop", "--listen", "127.0.0.1:0", "--url-file", urlFile],
    { stdio: "ignore" },
  );
  for (let i = 0; i < 100 && !(existsSync(urlFile) && readFileSync(urlFile, "utf8").trim()); i++) await sleep(100);
  url = readFileSync(urlFile, "utf8").trim();
});

test.afterAll(() => {
  server?.kill();
  rmSync(dir, { recursive: true, force: true });
});

test("frame time with about 2000 hosts", async ({ page }) => {
  await page.goto(`${url}&next=${encodeURIComponent("/?debug=1")}`);
  await waitReady(page);
  // Every host has spoken within the first 4 s of the capture.
  await expect.poll(() => page.evaluate(() => window.__cybercity!.stats().buildings), { timeout: 30_000 }).toBeGreaterThanOrEqual(1500);

  const frames = await page.evaluate(
    (ms) =>
      new Promise<number[]>((resolve) => {
        const deltas: number[] = [];
        let last = performance.now();
        const end = last + ms;
        const step = (now: number) => {
          deltas.push(now - last);
          last = now;
          if (now < end) requestAnimationFrame(step);
          else resolve(deltas);
        };
        requestAnimationFrame(step);
      }),
    SAMPLE_MS,
  );
  const stats = await page.evaluate(() => window.__cybercity!.stats());
  const sorted = frames.slice(1).sort((a, b) => a - b);
  const pct = (p: number) => sorted[Math.min(sorted.length - 1, Math.floor(p * sorted.length))] ?? 0;
  const result = {
    hosts: stats.buildings,
    frames: sorted.length,
    frame_ms_p50: Math.round(pct(0.5) * 10) / 10,
    frame_ms_p95: Math.round(pct(0.95) * 10) / 10,
    quality: stats.quality,
    renderer: stats.renderer,
    software: stats.software,
  };
  writeFileSync(join(ARTIFACTS, "perf.json"), `${JSON.stringify(result, null, 2)}\n`);
  console.log(`perf: ${JSON.stringify(result)}`);
  expect(sorted.length).toBeGreaterThan(0);
});
