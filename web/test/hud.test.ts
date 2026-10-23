import { describe, expect, it } from "vitest";
import { MovingRate, formatHosts, formatPps, formatRate } from "../src/ui/hud";

describe("HUD formatting", () => {
  it("formats rates with units", () => {
    expect(formatRate(0)).toBe("0 B/s");
    expect(formatRate(950)).toBe("950 B/s");
    expect(formatRate(1_234)).toBe("1.2 kB/s");
    expect(formatRate(45_600)).toBe("46 kB/s");
    expect(formatRate(2_500_000)).toBe("2.5 MB/s");
    expect(formatPps(42)).toBe("42 pkt/s");
    expect(formatPps(8_100)).toBe("8.1k pkt/s");
    expect(formatPps(2_000_000)).toBe("2.0M pkt/s");
  });

  it("shows hidden hosts only when there are some", () => {
    expect(formatHosts(40, 40, 0)).toBe("Hosts 40 / 40");
    expect(formatHosts(1990, 2010, 20)).toBe("Hosts 1990 / 2010 (+20 hidden)");
  });
});

describe("MovingRate", () => {
  it("averages the last second of ticks", () => {
    const m = new MovingRate();
    for (let i = 1; i <= 10; i++) m.add(i * 100, 100, { packets: 10, bytes: 1000 }); // 1 s of 100 ms ticks
    expect(m.rates()).toEqual({ pps: 100, bps: 10_000 });
    m.add(2_000, 100, { packets: 5, bytes: 500 }); // a heartbeat a second later
    expect(m.rates()).toEqual({ pps: 5, bps: 500 });
  });
});
