// SPDX-License-Identifier: MIT
// v1 wire protocol. Mirrors docs/PROTOCOL.md and internal/protocol; change all
// three together. Clients ignore unknown fields and unknown enum values.

export const PROTOCOL_VERSION = 1;

export const CATEGORIES = [
  "https",
  "web_cleartext",
  "dns",
  "ssh",
  "legacy_cleartext",
  "discovery",
  "control",
  "other",
] as const;
export type Category = (typeof CATEGORIES)[number];

export const DEVICE_CLASSES = ["workstation", "server", "iot", "network", "unknown"] as const;
export type DeviceClass = (typeof DEVICE_CLASSES)[number];

export const SECTORS = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"] as const;
export type Sector = (typeof SECTORS)[number];

export const EVENT_KINDS = ["port_scan", "flood", "host_idle"] as const;
export type EventKind = (typeof EVENT_KINDS)[number];

export interface ServerInfo {
  version: string;
  source: "live" | "file";
  iface: string | null;
  window_ms: number;
  anonymized: boolean;
  show_mac: boolean;
  geoip: boolean;
}

export interface District {
  id: string;
  name: string;
  kind: "lan" | "server" | "outlands";
  cidrs: string[];
}

export interface Service {
  port: number;
  proto: string;
  category: string;
  confirmed: boolean;
}

export interface Geo {
  sector: string;
  country: string;
}

export interface Host {
  id: string;
  ip: string | null;
  local: boolean;
  district: string;
  mac: string | null;
  vendor: string | null;
  device_class: string;
  services: Service[];
  geo: Geo | null;
  first_seen: number;
  last_seen: number;
  rtt_ms: number | null;
  retrans_rate: number;
}

export interface Flow {
  src: string;
  dst: string;
  proto: string;
  service_port: number | null;
  server_side: "src" | "dst" | null;
  category: string;
  packets: number;
  bytes: number;
  tcp_flags: string[];
  rtt_ms: number | null;
  retrans: number;
}

export interface Stats {
  packets: number;
  bytes: number;
  flows: number;
  hosts_total: number;
  kernel_drops: number;
  if_drops: number;
}

export interface ProtocolEvent {
  id: string;
  kind: string;
  ts: number;
  severity: "info" | "warn" | "alert";
  src: string | null;
  dst: string | null;
  targets: string[];
  evidence: Record<string, number>;
  duration_ms: number;
}

export interface Hello {
  v: 1;
  type: "hello";
  server: ServerInfo;
  districts: District[];
  hosts: Host[];
}

export interface Tick {
  v: 1;
  type: "tick";
  seq: number;
  ts: number;
  window_ms: number;
  stats: Stats;
  districts: District[];
  flows: Flow[];
  hosts: Host[];
  removed_hosts: string[];
  events: ProtocolEvent[];
}

export type Message = Hello | Tick;

/** Maps an unknown category to "other" (docs/PROTOCOL.md "Common conventions"). */
export function asCategory(c: string): Category {
  return (CATEGORIES as readonly string[]).includes(c) ? (c as Category) : "other";
}

// ---- Runtime validation -------------------------------------------------------

type Guard = (v: unknown) => boolean;

const isString: Guard = (v) => typeof v === "string";
const isNumber: Guard = (v) => typeof v === "number" && Number.isFinite(v);
const isInt: Guard = (v) => Number.isInteger(v);
const isBool: Guard = (v) => typeof v === "boolean";
const nullable =
  (g: Guard): Guard =>
  (v) =>
    v === null || g(v);
const arrayOf =
  (g: Guard): Guard =>
  (v) =>
    Array.isArray(v) && v.every(g);
const literal =
  (...values: readonly unknown[]): Guard =>
  (v) =>
    values.includes(v);

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

/** A shape lists a guard for every field of T: no more, no fewer (checked by the compiler). */
export type Shape<T> = { readonly [K in keyof T]-?: Guard };

function objectOf<T>(shape: Shape<T>): Guard {
  return (v) => isRecord(v) && Object.entries(shape).every(([k, g]) => (g as Guard)(v[k]));
}

const evidence: Guard = (v) => isRecord(v) && Object.values(v).every(isNumber);

export const SHAPES = {
  server: {
    version: isString,
    source: literal("live", "file"),
    iface: nullable(isString),
    window_ms: isInt,
    anonymized: isBool,
    show_mac: isBool,
    geoip: isBool,
  } satisfies Shape<ServerInfo>,
  district: {
    id: isString,
    name: isString,
    kind: literal("lan", "server", "outlands"),
    cidrs: arrayOf(isString),
  } satisfies Shape<District>,
  service: { port: isInt, proto: isString, category: isString, confirmed: isBool } satisfies Shape<Service>,
  geo: { sector: isString, country: isString } satisfies Shape<Geo>,
  stats: {
    packets: isInt,
    bytes: isInt,
    flows: isInt,
    hosts_total: isInt,
    kernel_drops: isInt,
    if_drops: isInt,
  } satisfies Shape<Stats>,
  flow: {
    src: isString,
    dst: isString,
    proto: isString,
    service_port: nullable(isInt),
    server_side: nullable(literal("src", "dst")),
    category: isString,
    packets: isInt,
    bytes: isInt,
    tcp_flags: arrayOf(isString),
    rtt_ms: nullable(isNumber),
    retrans: isInt,
  } satisfies Shape<Flow>,
  event: {
    id: isString,
    kind: isString,
    ts: isInt,
    severity: literal("info", "warn", "alert"),
    src: nullable(isString),
    dst: nullable(isString),
    targets: arrayOf(isString),
    evidence,
    duration_ms: isInt,
  } satisfies Shape<ProtocolEvent>,
};

const hostShape = {
  id: isString,
  ip: nullable(isString),
  local: isBool,
  district: isString,
  mac: nullable(isString),
  vendor: nullable(isString),
  device_class: isString,
  services: arrayOf(objectOf(SHAPES.service)),
  geo: nullable(objectOf(SHAPES.geo)),
  first_seen: isInt,
  last_seen: isInt,
  rtt_ms: nullable(isNumber),
  retrans_rate: isNumber,
} satisfies Shape<Host>;

const helloShape = {
  v: literal(PROTOCOL_VERSION),
  type: literal("hello"),
  server: objectOf(SHAPES.server),
  districts: arrayOf(objectOf(SHAPES.district)),
  hosts: arrayOf(objectOf(hostShape)),
} satisfies Shape<Hello>;

const tickShape = {
  v: literal(PROTOCOL_VERSION),
  type: literal("tick"),
  seq: isInt,
  ts: isInt,
  window_ms: isInt,
  stats: objectOf(SHAPES.stats),
  districts: arrayOf(objectOf(SHAPES.district)),
  flows: arrayOf(objectOf(SHAPES.flow)),
  hosts: arrayOf(objectOf(hostShape)),
  removed_hosts: arrayOf(isString),
  events: arrayOf(objectOf(SHAPES.event)),
} satisfies Shape<Tick>;

export const ALL_SHAPES = { ...SHAPES, host: hostShape, hello: helloShape, tick: tickShape };

export const isHello = objectOf(helloShape) as (v: unknown) => v is Hello;
export const isTick = objectOf(tickShape) as (v: unknown) => v is Tick;

/** Parses one WebSocket frame; returns null for anything that is not a valid v1 message. */
export function parseMessage(raw: string): Message | null {
  let data: unknown;
  try {
    data = JSON.parse(raw);
  } catch {
    return null;
  }
  if (isHello(data)) return data;
  if (isTick(data)) return data;
  return null;
}
