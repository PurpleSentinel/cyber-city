import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { STATE } from "./global-setup";

export default function globalTeardown(): void {
  const pidFile = join(STATE, "pid");
  if (!existsSync(pidFile)) return;
  try {
    process.kill(Number(readFileSync(pidFile, "utf8")));
  } catch {
    // already gone
  }
}
