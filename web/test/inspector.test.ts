import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import type { Hello, Host } from "../src/protocol";
import { inspectorRows, tooltipText } from "../src/ui/inspector";

const hello = JSON.parse(readFileSync(new URL("../../testdata/protocol/hello.json", import.meta.url), "utf8")) as Hello;
const host = hello.hosts[0]!;

describe("inspector rows", () => {
  it("shows the host's details with the class labelled heuristic", () => {
    const rows = Object.fromEntries(inspectorRows(host, "Corporate", 1_234_000));
    expect(rows).toMatchObject({
      Host: "10.0.1.10",
      Vendor: "Raspberry Pi Foundation",
      Class: "iot (heuristic)",
      District: "Corporate",
      Services: "22/tcp ssh",
      Traffic: "1.2 MB/s",
      RTT: "11.8 ms",
      Retransmits: "0.0%",
    });
    expect(rows["First seen"]).toMatch(/^\d\d:\d\d:\d\d$/);
  });

  it("handles anonymised hosts and missing values", () => {
    const anon: Host = { ...host, id: "anon-1234abcd", ip: null, vendor: null, rtt_ms: null, services: [] };
    const rows = Object.fromEntries(inspectorRows(anon, "LAN 1", 0));
    expect(rows.Host).toBe("anon-1234abcd");
    expect(rows.Vendor).toBe("—");
    expect(rows.RTT).toBe("—");
    expect(rows.Services).toBe("—");
    expect(tooltipText(anon)).toBe("anon-1234abcd");
    expect(tooltipText(host)).toBe("10.0.1.10 · Raspberry Pi Foundation");
  });
});
