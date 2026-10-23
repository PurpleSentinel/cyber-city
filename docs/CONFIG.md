# Configuration and Heuristics

The single source for flags, defaults, thresholds and classification rules. The Go defaults live in `internal/config/defaults.go`, and a unit test checks them against `config/cybercity.example.yaml`. If a value changes, change this file, the code and the example in the same PR.

Precedence: built-in defaults < config file (`--config`) < command-line flags.

## Command-line flags

| Flag | Default | Meaning |
|---|---|---|
| `--config <path>` | none | YAML config file (format: `config/cybercity.example.yaml`). |
| `--listen <host:port>` | `127.0.0.1:8080` | HTTP/WebSocket address. A non-loopback host logs a `WARN` explaining that anyone who obtains the link can view your traffic metadata. Port `0` picks a free port. |
| `--allow-origin <origin>` | none, repeatable | Extra allowed `Origin`s, e.g. `http://localhost:5173` for the Vite dev server. When set, the dev URL is printed too. |
| `--url-file <path>` | none | Write the ready-to-open `/auth?token=` URL to this file (mode 0600) once listening. Used by tests. |
| `--iface <name>` | default-route interface | Live capture interface. |
| `--list-ifaces` | | Print interfaces (name, addresses, which is the default route) and exit. |
| `--read <file.pcap>` | none | Replay a capture file instead of live capture. Mutually exclusive with `--iface`. |
| `--replay-speed <x>` | `1` | Replay speed multiplier; `0` = as fast as possible. |
| `--loop` | false | Restart the file at EOF. Timestamps are shifted so time keeps increasing. |
| `--dump-json <path\|->` | none | Write `hello` + every `tick` as JSON Lines and exit at EOF; no HTTP server. Implies `--replay-speed 0`. Requires `--read`. |
| `--bpf <expr>` | none | Capture filter (live: libpcap; file: `pcap.NewBPF`, which needs libpcap; without it, an error explains the limitation). |
| `--snaplen <n>` | `256` | Bytes captured per packet (DECISIONS D8). Range 128–65535. |
| `--no-promisc` | false | Disable promiscuous mode. |
| `--user <name>` | `nobody` when started as root | Drop to this user after opening the capture handle. `--user root` keeps root and logs a `WARN`. |
| `--show-mac` | false | Include MAC addresses in Host objects. |
| `--anonymize` | false | Pseudonymous host ids, `ip: null`, anonymous districts (PROTOCOL.md). |
| `--districts <path>` | none | District map (format below). |
| `--geoip <path.mmdb>` | none | GeoLite2-City database. Requires `geo.home` in the config file. |
| `--window <dur>` | `100ms` | Aggregation window. Allowed range 50ms–1s. |
| `--max-hosts <n>` | `2000` | Tracked hosts; least-recently-seen evicted beyond this. |
| `--log-level <lvl>` | `info` | `debug \| info \| warn \| error`, via `log/slog` to stderr. |
| `--version` | | Print version and exit. |

Startup order (privilege handling depends on it; DECISIONS D22):
1. Parse flags and config; load the districts and GeoIP files.
2. Open the capture (or the `--dump-json` output file).
3. Bind the listener and write `--url-file` (handed to `SUDO_UID` when run under sudo).
4. Drop privileges when root (`setgroups([])`, `setgid`, `setuid`, then verify root cannot be regained; abort on failure).
5. Serve HTTP and print the URL.

## Config file keys and defaults

See `config/cybercity.example.yaml`. Keys not listed in the flag table:

| Key | Default | Meaning |
|---|---|---|
| `server.max_clients` | `4` | Further WebSocket upgrades get `503`. |
| `server.client_queue` | `32` | Messages buffered per client before it is dropped. |
| `server.write_timeout` | `2s` | Per-message write deadline. |
| `server.ping_interval` | `15s` | |
| `server.heartbeat` | `1s` | Maximum gap between ticks. |
| `flows.max_tracked` | `50000` | Flow/handshake state entries; oldest evicted first. |
| `hosts.idle_timeout` | `120s` | No packets from a host for this long → removed (local hosts also emit `host_idle`). |
| `hosts.max_services` | `16` | |
| `enrich.handshake_timeout` | `10s` | Pending SYNs older than this are discarded (they also count as "incomplete" for the scan detector). |
| `enrich.rtt_samples` | `20` | Host RTT median window. |
| `enrich.retrans_window` | `30s` | |
| `enrich.class_interval` | `10s` | How often device class is re-evaluated. |
| `enrich.gateway_min_ips` | `5` | DECISIONS D13. |
| `detect.scan.window` | `10s` | |
| `detect.scan.min_ports` | `50` | Distinct ports on one target. |
| `detect.scan.min_hosts` | `20` | Distinct targets on one port. |
| `detect.scan.min_incomplete_ratio` | `0.8` | |
| `detect.scan.udp_min_ports` | `100` | |
| `detect.scan.cooldown` | `60s` | Per source. |
| `detect.flood.window` | `5s` | |
| `detect.flood.baseline_half_life` | `60s` | EWMA of per-target packets/s. |
| `detect.flood.min_history` | `30s` | No flood events for a target with less history than this. |
| `detect.flood.multiplier` | `10` | |
| `detect.flood.min_pps` | `2000` | |
| `detect.flood.min_sources` | `50` | Set to `1` to also flag single-source floods. |
| `detect.flood.cooldown` | `60s` | Per target. |
| `geo.home` | none | `[lat, lon]` of the sensor; bearings are measured from here. |

## Districts file

```yaml
# config/districts.example.yaml
districts:
  - id: core            # [a-z0-9-]{1,32}, unique; "outlands" is reserved
    name: Server Core
    kind: server        # lan | server
    cidrs: [10.0.9.0/24]
    vlans: [20]         # optional 802.1Q VLAN ids
  - id: corp
    name: Corporate
    kind: lan
    cidrs: [10.0.1.0/24, fd00:1::/64]
    vlans: [10]
```

Matching for a local host: the first district whose `vlans` contains the packet's VLAN id, else the first whose `cidrs` contains the IP, else an auto district (PROTOCOL.md). Non-local hosts always go to `outlands`. A host's district is fixed when it is first seen.

## Service port and server side

Decided once per 5-tuple and cached for that flow's lifetime:
1. A TCP SYN without ACK was seen: server = SYN destination.
2. Else a SYN+ACK was seen: server = SYN-ACK source.
3. Else exactly one of the two ports is in the category table below: that side is the server.
4. Else, if either port is < 1024: the lower port is the server.
5. Else: `service_port` = the lower port, `server_side` = `null`.

## Traffic categories

| Category | Ports | Colour (VISUAL_SPEC) |
|---|---|---|
| `https` | tcp 443, 8443; udp 443 (QUIC) | green |
| `web_cleartext` | tcp 80, 8000, 8080 | orange |
| `dns` | udp/tcp 53; tcp 853 | yellow |
| `ssh` | tcp 22 | white |
| `legacy_cleartext` | tcp 21, 23, 25, 110, 143, 512, 513, 514; udp 69, 161 | red |
| `discovery` | udp 67, 68, 137, 138, 546, 547, 1900, 5353, 5355 | violet |
| `control` | ICMP, ICMPv6 | grey-blue |
| `other` | anything else | grey |

The bit index for the facade-window mask is the row order above, starting at 0 (`https` = bit 0 … `other` = bit 7).

## Device class

Re-evaluated every `enrich.class_interval`; the first rule that matches wins. Vendor matching is a case-insensitive substring match against the OUI vendor name.

1. **network**: the host is a detected gateway (D13), or its vendor contains one of: Cisco, Juniper, Ubiquiti, MikroTik, Aruba, Netgear, TP-Link, Fortinet, Palo Alto, Zyxel, Arista.
2. **iot**: its vendor contains one of: Espressif, Tuya, Shelly, Allterco, Signify, Philips Lighting, Sonos, Hikvision, Dahua, Xiaomi, Amazon Technologies, Nest Labs, Roku, Ring, Wyze, Raspberry Pi; or it is a client of tcp 1883/8883 (MQTT).
3. **server**: ≥ 1 confirmed service, and ≥ 3 distinct peers connected to its services in the last 5 minutes.
4. **workstation**: local, and it opened `https` connections to ≥ 3 distinct non-local hosts in the last 5 minutes.
5. **unknown**.

## Enrichment details

- **Handshake RTT**: for a TCP 4-tuple, record the SYN timestamp; on the client's first ACK (no SYN) after a SYN-ACK, `rtt = ack_ts − syn_ts`. This is the full client↔server round trip as seen from the sensor, wherever the sensor sits. Ignore it if a SYN was retransmitted.
- **Retransmission**: per TCP direction, track the highest `seq + payload_len` seen. A segment with payload > 0 whose `seq + len ≤` that value is a retransmission. Use the IP header's lengths, not the captured length, because snaplen truncates payloads.
- **Local address**: `net.IP.IsPrivate() || IsLoopback() || IsLinkLocalUnicast()`. This covers fc00::/7 ULA and fe80::/10.
- **Geo sector**: the initial great-circle bearing from `geo.home` to the IP's GeoLite2 location, quantised to 8 sectors centred on N (0°), NE (45°), …. No location → `geo: null`.

## Event detectors

- **Port scan** (per source, sliding `detect.scan.window`): a probe is a SYN to `(dst, port)`. It is *incomplete* if the reply is RST or there is no SYN-ACK within `enrich.handshake_timeout` (evaluated at window end, so a probe counts as incomplete if no SYN-ACK has been seen by then). Fire if (distinct ports on one dst ≥ `min_ports` or distinct dsts on one port ≥ `min_hosts`) and incomplete/total ≥ `min_incomplete_ratio`. UDP: distinct dst ports ≥ `udp_min_ports` from one source to one dst. `targets` = probed hosts (max 64).
- **Flood** (per destination): baseline = an EWMA of packets/s with `baseline_half_life`, updated each window and **frozen while a flood is active**. Fire if history ≥ `min_history` and packets/s over the last `window` ≥ max(`multiplier` × baseline, `min_pps`) and distinct sources in that window ≥ `min_sources`.
- **Host idle**: a local host with no packets for `hosts.idle_timeout`.
