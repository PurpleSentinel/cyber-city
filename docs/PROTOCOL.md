# Wire Protocol (v1)

The contract between the Go backend and the browser. Go types in `internal/protocol` and TypeScript types in `web/src/protocol.ts` mirror this document exactly. Change all three in the same PR.

## Transport and endpoints

All endpoints are served by the `cybercity` binary on one origin (default `http://127.0.0.1:8080`).

| Method + path | Auth | Behaviour |
|---|---|---|
| `GET /auth?token=<T>[&next=<path>]` | token | Constant-time compare with the per-run token. Valid: set cookie `cc_session=<random 32 bytes, base64url>; HttpOnly; SameSite=Strict; Path=/` and `302` to `next` if it starts with `/` and contains no `//` or `\`, else to `/`. Invalid: `401`, plain-text body. The token remains valid for the whole run, so the link can be reopened. |
| `GET /` and static assets | none | Embedded frontend. Contains no data. |
| `GET /api/session` | cookie | `204` if the session is valid, else `401`. The frontend calls this before opening the socket so it can say "invalid or expired link" instead of failing silently. |
| `GET /ws` (WebSocket upgrade) | cookie + Origin | `401` without a valid session; `403` if `Origin` is not allowed. Allowed origins: `http://<listen address>` plus each `--allow-origin`. |

Security headers on every response:
```
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'
X-Content-Type-Options: nosniff
Referrer-Policy: no-referrer
Cache-Control: no-store            (for /, /auth, /api/*)
```

WebSocket rules:
- Text frames, one JSON object per frame, UTF-8.
- The server sends `hello` first, then `tick`s.
- The client sends nothing. The server's read limit is 512 bytes; a larger message closes the connection.
- Server pings every 15 s; a client missing 2 pongs is dropped.
- Each client has a bounded queue (`server.client_queue`, default 32). If it is full, the client is closed with code `1013` (try again later) rather than stalling others.

## Common conventions

- `v` is always `1`. Additive changes (new optional fields, new event kinds, new categories) do **not** bump it. Removing or changing the meaning of a field does.
- Clients ignore unknown fields, unknown event `kind`s and unknown `category` values (render as `other`).
- Times are integers: milliseconds since the Unix epoch, taken from packet timestamps (DECISIONS D6).
- Optional values are `null`, never omitted. Arrays are `[]` when empty, never `null`.
- **Host ids** are the canonical IP string (`10.0.1.10`, `fd00:1::10`), or `anon-<8 hex>` under `--anonymize` (first 8 hex digits of HMAC-SHA256 of the IP under a random per-run key).
- **Ordering** (DECISIONS D16): `flows` by `bytes` descending, then `src`, `dst`, `service_port`; `hosts` and `removed_hosts` by `id`; `events` by `ts`, then `id`; `services` by `port`, then `proto`; `districts` by `id`; `targets` by `id`.

## `hello`

Sent once per connection: the configuration plus a full snapshot.

```json
{
  "v": 1,
  "type": "hello",
  "server": {
    "version": "0.1.0",
    "source": "live",
    "iface": "enp3s0",
    "window_ms": 100,
    "anonymized": false,
    "show_mac": false,
    "geoip": false
  },
  "districts": [
    { "id": "core", "name": "Server Core", "kind": "server", "cidrs": ["10.0.9.0/24"] },
    { "id": "corp", "name": "Corporate",   "kind": "lan",    "cidrs": ["10.0.1.0/24"] },
    { "id": "outlands", "name": "The Outlands", "kind": "outlands", "cidrs": [] }
  ],
  "hosts": [ /* Host objects, as in tick */ ]
}
```

- `server.source`: `"live"` or `"file"`; `iface` is `null` for files.
- `district.kind`: `"lan" | "server" | "outlands"`. Exactly one `outlands` district, id `outlands`, always present.
- Auto districts (no config match): one per private /24 (/64 for IPv6), id `lan-10.0.1` / `lan-fd00-1-0-0`, name `LAN 10.0.1.0/24`. Under `--anonymize`: ids `lan-1`, `lan-2`, … in order of first sight, names `LAN 1`, …, and `cidrs: []` for every district.
- A district first seen after `hello` is announced through a tick's `districts` array.

## `tick`

Sent once per aggregation window that saw at least one packet, and at least every 1000 ms (an empty heartbeat tick) so the client can show liveness and decay heights.

```json
{
  "v": 1,
  "type": "tick",
  "seq": 4182,
  "ts": 1767225610100,
  "window_ms": 100,
  "stats": {
    "packets": 812, "bytes": 603112, "flows": 37,
    "hosts_total": 142, "kernel_drops": 0, "if_drops": 0
  },
  "districts": [],
  "flows": [ /* Flow */ ],
  "hosts": [ /* Host: new or changed since the previous tick */ ],
  "removed_hosts": ["203.0.113.77"],
  "events": [ /* Event */ ]
}
```

- `seq` increases by 1 per tick per connection, starting at 1.
- `ts` is the end of the window.
- `stats.kernel_drops` / `if_drops` are cumulative pcap counters (always 0 for files).
- A host is "changed" if any Host field other than `last_seen` changed, or if `last_seen` advanced by ≥ 5 s since it was last sent.

### Flow

Traffic between two hosts during this window, keyed by `(proto, src, dst, service_port)`. Direction is client → server where a server side is known.

```json
{
  "src": "10.0.1.10",
  "dst": "203.0.113.10",
  "proto": "tcp",
  "service_port": 443,
  "server_side": "dst",
  "category": "https",
  "packets": 12,
  "bytes": 9812,
  "tcp_flags": ["ACK", "PSH"],
  "rtt_ms": 12.1,
  "retrans": 0
}
```

| Field | Type | Notes |
|---|---|---|
| `src`, `dst` | host id | `src` is the client if `server_side` is `"dst"`. `dst` may be a multicast or broadcast address with no Host object (DECISIONS D21). |
| `proto` | `"tcp" \| "udp" \| "icmp" \| "icmpv6" \| "other"` | |
| `service_port` | int or `null` | The port the server listens on (CONFIG.md, *Service port*). `null` for ICMP and other protocols. |
| `server_side` | `"src" \| "dst" \| null` | `null` if unknown. |
| `category` | string | Table in CONFIG.md, *Traffic categories*. |
| `packets`, `bytes` | int | Both directions combined; `bytes` is on-wire length. |
| `tcp_flags` | string[] | Union over the window, from `SYN, ACK, FIN, RST, PSH, URG`, in that order. `[]` for non-TCP. |
| `rtt_ms` | number or `null` | Most recent handshake RTT for this flow key, 1 decimal place. |
| `retrans` | int | Retransmitted data segments in this window. |

### Host

```json
{
  "id": "10.0.1.10",
  "ip": "10.0.1.10",
  "local": true,
  "district": "corp",
  "mac": null,
  "vendor": "Raspberry Pi Foundation",
  "device_class": "iot",
  "services": [ { "port": 22, "proto": "tcp", "category": "ssh", "confirmed": true } ],
  "geo": null,
  "first_seen": 1767225600000,
  "last_seen": 1767225610090,
  "rtt_ms": 11.8,
  "retrans_rate": 0.0
}
```

| Field | Type | Notes |
|---|---|---|
| `ip` | string or `null` | `null` under `--anonymize`. |
| `local` | bool | Private, loopback, link-local or ULA address. |
| `district` | district id | |
| `mac` | string or `null` | Lower-case colon form. Only with `--show-mac`, and only per DECISIONS D13. |
| `vendor` | string or `null` | From OUI, same D13 rule (vendor is sent even without `--show-mac`). |
| `device_class` | `"workstation" \| "server" \| "iot" \| "network" \| "unknown"` | Heuristic (CONFIG.md, *Device class*). |
| `services` | Service[] | Ports this host **serves**. `confirmed` is true once a SYN-ACK (TCP) or a reply from that port (UDP) is seen from this host. Max 16; on overflow, drop the least recently seen. |
| `geo` | `{ "sector": "N"\|"NE"\|"E"\|"SE"\|"S"\|"SW"\|"W"\|"NW", "country": "JP" }` or `null` | Only with `--geoip`, only for non-local hosts. |
| `rtt_ms` | number or `null` | Median handshake RTT over the last 20 handshakes involving this host. |
| `retrans_rate` | number | Retransmitted / total data segments sent by this host over the last 30 s, 0–1, 3 decimals. |

### Event

```json
{
  "id": "e-17",
  "kind": "port_scan",
  "ts": 1767225660000,
  "severity": "alert",
  "src": "10.0.1.66",
  "dst": "10.0.9.5",
  "targets": ["10.0.9.5"],
  "evidence": { "distinct_ports": 1024, "distinct_hosts": 1, "incomplete_ratio": 0.99, "window_s": 10 },
  "duration_ms": 6000
}
```

| `kind` | `severity` | `src` / `dst` | `evidence` fields |
|---|---|---|---|
| `port_scan` | `alert` | scanner / single target or `null` for sweeps | `distinct_ports`, `distinct_hosts`, `incomplete_ratio`, `window_s` |
| `flood` | `alert` | `null` (or the only source) / target | `pps`, `baseline_pps`, `distinct_sources`, `window_s` |
| `host_idle` | `info` | the host / `null` | `idle_s` |

- `id`: `e-<n>`, increasing per run.
- `targets`: at most 64 host ids, sorted.
- `duration_ms`: how long the client should play the effect (VISUAL_SPEC.md).
- A `host_idle` event for host X is followed in the **same tick** by X in `removed_hosts`.

## Client behaviour summary

1. `GET /api/session`: on `401`, show "Invalid or expired link — reopen the URL printed by cybercity" and stop.
2. Open `/ws` and wait for `hello`; build state from the snapshot.
3. Apply each `tick` in `seq` order. On a gap or a disconnect: reconnect with backoff, then rebuild from the new `hello`.
