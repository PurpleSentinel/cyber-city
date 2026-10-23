// SPDX-License-Identifier: MIT
// WebSocket connection manager (docs/PROTOCOL.md "Client behaviour"):
// session check → open /ws → hello → ticks in seq order; on a seq gap or a
// disconnect, reconnect with backoff and rebuild from the next hello.
import type { Hello, Tick } from "../protocol";
import { parseMessage } from "../protocol";
import { backoffDelayMs } from "./backoff";
import type { SessionState } from "./session";
import { checkSession } from "./session";

export type ConnectionState =
  | { kind: "connecting" }
  | { kind: "live" }
  | { kind: "reconnecting"; inMs: number }
  | { kind: "invalid" };

export interface ConnectionHandlers {
  onHello(hello: Hello): void;
  onTick(tick: Tick): void;
  onState(state: ConnectionState): void;
}

/** The subset of WebSocket used here, so tests can substitute a fake. */
export interface SocketLike {
  onopen: (() => void) | null;
  onclose: (() => void) | null;
  onmessage: ((ev: { data: string }) => void) | null;
  close(): void;
}

export interface ConnectionDeps {
  openSocket(url: string): SocketLike;
  checkSession(): Promise<SessionState>;
  setTimeout(fn: () => void, ms: number): unknown;
  wsUrl: string;
  random?: () => number;
}

export function defaultDeps(): ConnectionDeps {
  const url = new URL("/ws", window.location.href);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return {
    openSocket: (u) => new WebSocket(u) as unknown as SocketLike,
    checkSession: () => checkSession(),
    setTimeout: (fn, ms) => window.setTimeout(fn, ms),
    wsUrl: url.toString(),
  };
}

export class Connection {
  private socket: SocketLike | null = null;
  private lastSeq = 0;
  private gotHello = false;
  private attempt = 0;
  private stopped = false;

  constructor(
    private readonly handlers: ConnectionHandlers,
    private readonly deps: ConnectionDeps,
  ) {}

  async start(): Promise<void> {
    this.handlers.onState({ kind: "connecting" });
    const session = await this.deps.checkSession();
    if (session === "invalid") {
      this.stopped = true;
      this.handlers.onState({ kind: "invalid" });
      return;
    }
    if (session === "unreachable") {
      this.scheduleReconnect();
      return;
    }
    this.open();
  }

  /** Drops the socket and reconnects for a fresh hello (e.g. after a pause). */
  resync(): void {
    this.reconnectNow();
  }

  stop(): void {
    this.stopped = true;
    this.socket?.close();
  }

  private open(): void {
    const s = this.deps.openSocket(this.deps.wsUrl);
    this.socket = s;
    this.lastSeq = 0;
    this.gotHello = false;
    s.onmessage = (ev) => this.onMessage(ev.data);
    s.onclose = () => {
      if (this.socket === s) {
        this.socket = null;
        this.scheduleReconnect();
      }
    };
  }

  private onMessage(raw: string): void {
    const msg = parseMessage(raw);
    if (!msg) return;
    if (msg.type === "hello") {
      this.gotHello = true;
      this.lastSeq = 0;
      this.attempt = 0;
      this.handlers.onHello(msg);
      this.handlers.onState({ kind: "live" });
      return;
    }
    if (!this.gotHello) return;
    if (msg.seq !== this.lastSeq + 1) {
      // Missed ticks: state is no longer trustworthy. Reconnect for a new hello.
      this.reconnectNow();
      return;
    }
    this.lastSeq = msg.seq;
    this.handlers.onTick(msg);
  }

  private reconnectNow(): void {
    const s = this.socket;
    this.socket = null;
    s?.close();
    this.scheduleReconnect();
  }

  private scheduleReconnect(): void {
    if (this.stopped) return;
    const inMs = backoffDelayMs(this.attempt++, this.deps.random);
    this.handlers.onState({ kind: "reconnecting", inMs });
    this.deps.setTimeout(() => {
      void this.retry();
    }, inMs);
  }

  private async retry(): Promise<void> {
    if (this.stopped) return;
    const session = await this.deps.checkSession();
    if (session === "invalid") {
      this.stopped = true;
      this.handlers.onState({ kind: "invalid" });
      return;
    }
    if (session === "unreachable") {
      this.scheduleReconnect();
      return;
    }
    this.handlers.onState({ kind: "connecting" });
    this.open();
  }
}
