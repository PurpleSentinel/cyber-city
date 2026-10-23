// SPDX-License-Identifier: MIT
// Client-side view of the city, rebuilt from each hello and updated by ticks
// (docs/PROTOCOL.md "Client behaviour"). Renderers subscribe to changes.
import { HUD } from "../config";
import type { District, Flow, Hello, Host, ProtocolEvent, ServerInfo, Stats, Tick } from "../protocol";

export interface StoreListener {
  reset?(): void;
  districtAdded?(d: District): void;
  hostAdded?(h: Host): void;
  hostUpdated?(h: Host, prev: Host): void;
  hostRemoved?(id: string, viaEvent: boolean): void;
  tick?(t: Tick): void;
  event?(e: ProtocolEvent): void;
}

export class Store {
  server: ServerInfo | null = null;
  readonly districts = new Map<string, District>();
  readonly hosts = new Map<string, Host>();
  lastStats: Stats | null = null;
  lastFlows: readonly Flow[] = [];
  ticks = 0;
  /** Most recent events, newest first, capped at HUD.eventLogSize. */
  events: ProtocolEvent[] = [];
  private readonly listeners: StoreListener[] = [];

  subscribe(l: StoreListener): void {
    this.listeners.push(l);
  }

  applyHello(h: Hello): void {
    this.server = h.server;
    this.districts.clear();
    this.hosts.clear();
    this.ticks = 0;
    this.lastFlows = [];
    for (const l of this.listeners) l.reset?.();
    for (const d of h.districts) this.addDistrict(d);
    for (const host of h.hosts) this.upsertHost(host);
  }

  applyTick(t: Tick): void {
    this.ticks++;
    this.lastStats = t.stats;
    this.lastFlows = t.flows;
    for (const d of t.districts) this.addDistrict(d);
    for (const host of t.hosts) this.upsertHost(host);
    // host_idle events precede their removal so the outage effect can play.
    const idle = new Set(t.events.filter((e) => e.kind === "host_idle" && e.src).map((e) => e.src as string));
    for (const e of t.events) {
      this.events.unshift(e);
      for (const l of this.listeners) l.event?.(e);
    }
    if (this.events.length > HUD.eventLogSize) this.events.length = HUD.eventLogSize;
    for (const id of t.removed_hosts) {
      if (this.hosts.delete(id)) {
        for (const l of this.listeners) l.hostRemoved?.(id, idle.has(id));
      }
    }
    for (const l of this.listeners) l.tick?.(t);
  }

  private addDistrict(d: District): void {
    if (this.districts.has(d.id)) return;
    this.districts.set(d.id, d);
    for (const l of this.listeners) l.districtAdded?.(d);
  }

  private upsertHost(h: Host): void {
    const prev = this.hosts.get(h.id);
    this.hosts.set(h.id, h);
    for (const l of this.listeners) {
      if (prev) l.hostUpdated?.(h, prev);
      else l.hostAdded?.(h);
    }
  }
}
