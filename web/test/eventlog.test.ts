// SPDX-License-Identifier: MIT
// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import type { ProtocolEvent } from "../src/protocol";
import { EVENT_LOG_FOOTER, EventLog, formatEvidence, formatParties, formatTime, logEntries } from "../src/ui/eventlog";

const ev = (n: number, extra: Partial<ProtocolEvent> = {}): ProtocolEvent => ({
  id: `e-${n}`,
  kind: "port_scan",
  ts: 1767225660000 + n * 1000,
  severity: "alert",
  src: "10.0.1.66",
  dst: "10.0.9.5",
  targets: ["10.0.9.5"],
  evidence: { distinct_ports: 1024, distinct_hosts: 1, incomplete_ratio: 0.99, window_s: 10 },
  duration_ms: 6000,
  ...extra,
});

describe("event log formatting", () => {
  it("formats time, parties and evidence per kind", () => {
    const d = new Date(1767225660000);
    const two = (x: number) => String(x).padStart(2, "0");
    expect(formatTime(d.getTime())).toBe(`${two(d.getHours())}:${two(d.getMinutes())}:${two(d.getSeconds())}`);

    expect(formatParties(ev(1))).toBe("10.0.1.66 → 10.0.9.5");
    const sweep = ev(2, { dst: null, targets: ["10.0.1.2", "10.0.1.4", "10.0.1.6", "10.0.1.8", "10.0.1.10"] });
    expect(formatParties(sweep)).toBe("10.0.1.66 → 10.0.1.2, 10.0.1.4, 10.0.1.6 +2");
    expect(formatEvidence(ev(1))).toBe("1024 ports · 99% incomplete");
    expect(formatEvidence({ ...sweep, evidence: { distinct_ports: 1, distinct_hosts: 20, incomplete_ratio: 1, window_s: 10 } })).toBe(
      "20 hosts · 100% incomplete",
    );

    const flood = ev(3, { kind: "flood", src: null, evidence: { pps: 3000, baseline_pps: 50.4, distinct_sources: 60, window_s: 5 } });
    expect(formatParties(flood)).toBe("many → 10.0.9.5");
    expect(formatEvidence(flood)).toBe("3000 pkt/s (baseline 50) · 60 sources");

    const idle = ev(4, { kind: "host_idle", severity: "info", src: "10.0.1.30", dst: null, targets: [], evidence: { idle_s: 120 } });
    expect(formatParties(idle)).toBe("10.0.1.30");
    expect(formatEvidence(idle)).toBe("idle 120 s");
    expect(logEntries([idle])[0]?.kind).toBe("Host idle");
  });
});

describe("EventLog", () => {
  it("renders the newest 50 entries with the not-an-IDS footer", () => {
    const log = new EventLog(document);
    const events = Array.from({ length: 60 }, (_, i) => ev(60 - i)); // newest first
    log.render(events);
    const items = document.querySelectorAll("#eventlog li");
    expect(items).toHaveLength(50);
    expect(items[0]?.getAttribute("data-event")).toBe("e-60");
    expect(items[0]?.className).toBe("alert");
    expect(items[0]?.querySelector(".kind")?.textContent).toBe("Port scan");
    expect(items[0]?.querySelector(".parties")?.textContent).toBe("10.0.1.66 → 10.0.9.5");
    expect(document.querySelector("#eventlog .footer")?.textContent).toBe(EVENT_LOG_FOOTER);
    expect(EVENT_LOG_FOOTER).toBe("Heuristic detections — not an IDS");
    log.render([]);
    expect(document.querySelectorAll("#eventlog li")).toHaveLength(0);
  });

  it("writes text only, never markup from the wire", () => {
    const log = new EventLog(document);
    log.render([ev(1, { src: "<img src=x onerror=alert(1)>" })]);
    expect(document.querySelector("#eventlog img")).toBeNull();
  });
});
