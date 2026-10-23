import { spawn } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";

export const ROOT = join(import.meta.dirname, "..", "..");
export const STATE = join(import.meta.dirname, ".state");

/** Starts the real binary replaying the demo capture (docs/TESTING.md step 1). */
export default async function globalSetup(): Promise<void> {
  const bin = join(ROOT, "bin", "cybercity");
  if (!existsSync(bin)) throw new Error("bin/cybercity missing: run `make build` first (make e2e does)");
  mkdirSync(STATE, { recursive: true });
  const urlFile = join(STATE, "url.txt");
  rmSync(urlFile, { force: true });
  const child = spawn(
    bin,
    ["--read", join(ROOT, "testdata", "pcap", "demo.pcap"), "--replay-speed", "4", "--loop", "--listen", "127.0.0.1:0", "--url-file", urlFile],
    { stdio: "ignore", detached: false },
  );
  writeFileSync(join(STATE, "pid"), String(child.pid));
  for (let i = 0; i < 100; i++) {
    if (existsSync(urlFile) && readFileSync(urlFile, "utf8").trim() !== "") return;
    await sleep(100);
  }
  child.kill();
  throw new Error("cybercity did not write its URL within 10 s");
}
