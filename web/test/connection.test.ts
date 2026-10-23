// SPDX-License-Identifier: MIT
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { backoffDelayMs } from "../src/net/backoff";
import type { ConnectionState, SocketLike } from "../src/net/connection";
import { Connection } from "../src/net/connection";
import type { SessionState } from "../src/net/session";

const hello = readFileSync(new URL("../../testdata/protocol/hello.json", import.meta.url), "utf8");
const tickJSON = JSON.parse(readFileSync(new URL("../../testdata/protocol/tick.json", import.meta.url), "utf8")) as Record<string, unknown>;
const tick = (seq: number): string => JSON.stringify({ ...tickJSON, seq });

class FakeSocket implements SocketLike {
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onmessage: ((ev: { data: string }) => void) | null = null;
  closed = false;
  close(): void {
    this.closed = true;
    this.onclose?.();
  }
  send(data: string): void {
    this.onmessage?.({ data });
  }
}

function harness(sessions: SessionState[]) {
  const sockets: FakeSocket[] = [];
  const timers: { fn: () => void; ms: number }[] = [];
  const states: ConnectionState[] = [];
  const hellos: number[] = [];
  const ticks: number[] = [];
  const conn = new Connection(
    {
      onHello: (h) => hellos.push(h.hosts.length),
      onTick: (t) => ticks.push(t.seq),
      onState: (s) => states.push(s),
    },
    {
      openSocket: () => {
        const s = new FakeSocket();
        sockets.push(s);
        return s;
      },
      checkSession: async () => sessions.shift() ?? "valid",
      setTimeout: (fn, ms) => timers.push({ fn, ms }),
      wsUrl: "ws://test/ws",
      random: () => 0.5, // no jitter
    },
  );
  return { conn, sockets, timers, states, hellos, ticks };
}

const flush = () => new Promise((r) => setTimeout(r, 0));

describe("backoff", () => {
  it("doubles from 0.5 s up to 10 s with ±20% jitter", () => {
    const mid = () => 0.5;
    expect([0, 1, 2, 3, 4, 5, 9].map((n) => backoffDelayMs(n, mid))).toEqual([500, 1000, 2000, 4000, 8000, 10000, 10000]);
    expect(backoffDelayMs(0, () => 0)).toBe(400);
    expect(backoffDelayMs(0, () => 0.999999)).toBe(600);
  });
});

describe("Connection", () => {
  it("applies hello then ticks in order", async () => {
    const h = harness(["valid"]);
    await h.conn.start();
    const s = h.sockets[0]!;
    s.send(hello);
    s.send(tick(1));
    s.send(tick(2));
    expect(h.hellos).toEqual([2]);
    expect(h.ticks).toEqual([1, 2]);
    expect(h.states.at(-1)).toEqual({ kind: "live" });
  });

  it("reconnects on a seq gap and rebuilds from the new hello", async () => {
    const h = harness(["valid", "valid"]);
    await h.conn.start();
    const first = h.sockets[0]!;
    first.send(hello);
    first.send(tick(1));
    first.send(tick(3)); // gap
    expect(first.closed).toBe(true);
    expect(h.ticks).toEqual([1]);
    expect(h.states.at(-1)).toEqual({ kind: "reconnecting", inMs: 500 });

    h.timers.shift()!.fn();
    await flush();
    const second = h.sockets[1]!;
    second.send(hello);
    second.send(tick(1)); // seq restarts per connection
    expect(h.hellos).toEqual([2, 2]);
    expect(h.ticks).toEqual([1, 1]);
  });

  it("backs off on repeated disconnects and resets after a hello", async () => {
    const h = harness(["valid", "valid", "valid", "valid"]);
    await h.conn.start();
    h.sockets[0]!.close();
    expect(h.timers.at(-1)!.ms).toBe(500);
    h.timers.shift()!.fn();
    await flush();
    h.sockets[1]!.close();
    expect(h.timers.at(-1)!.ms).toBe(1000);
    h.timers.shift()!.fn();
    await flush();
    h.sockets[2]!.send(hello);
    h.sockets[2]!.close();
    expect(h.timers.at(-1)!.ms).toBe(500);
  });

  it("stops with an invalid-link state when the session is rejected", async () => {
    const h = harness(["invalid"]);
    await h.conn.start();
    expect(h.sockets).toHaveLength(0);
    expect(h.states.at(-1)).toEqual({ kind: "invalid" });
  });

  it("ignores ticks before hello", async () => {
    const h = harness(["valid"]);
    await h.conn.start();
    h.sockets[0]!.send(tick(1));
    expect(h.ticks).toEqual([]);
  });
});
