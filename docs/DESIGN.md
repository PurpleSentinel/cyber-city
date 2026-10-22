# CyberCity-3D Design

CyberCity-3D turns the metadata seen by a capture interface (IP and MAC addresses, protocols, ports, packet sizes, timing) into a living 3D city. Hosts are buildings, subnets are districts, conversations are highways, and notable network behaviour becomes a visual event. The city should grow, pulse, and decay with the network rather than accumulate static blocks.

Each mapping below carries a **Feasibility** note: what a passive sensor can actually derive. The project never inspects or transmits packet payloads, so anything that would need payload inspection is either reframed or listed as out of scope.

## What the sensor can see

Promiscuous mode passes every frame that reaches the interface to the capture process, but on a modern **switched** network only these frames reach it:

- broadcast and multicast traffic (ARP, mDNS, SSDP, DHCP, …)
- traffic to and from the capturing host itself

To see every host on a network, connect the capture interface to a **SPAN / mirror port** on a managed switch or to a **network TAP**. On Wi-Fi, promiscuous mode on an associated interface likewise shows mostly the host's own traffic; monitor mode is a separate topic and out of scope for now.

Capture needs `CAP_NET_RAW` / `CAP_NET_ADMIN`, not a full root session: grant the capabilities to the built binary with `setcap`, or start as root and drop privileges once the capture handle is open (see ROADMAP Milestone 1).

## Anatomy of the city

```
THE NETWORK ENVIRONMENT
 ├── DISTRICTS ............ subnets / VLANs / (optional) geolocation
 │    └── BUILDINGS ....... individual hosts (IP / MAC)
 │         ├── Windows .... active ports / services
 │         └── Shaders .... traffic volume, health, status
 └── HIGHWAYS ............. conversations between hosts
      └── Particle freight  packets, colour-coded by protocol
```

### Districts: urban zoning (subnets and VLANs)

| District | Network meaning | Look |
|---|---|---|
| **Corporate / LAN** | Workstation subnets, e.g. `10.0.1.0/24` | Dense, orderly grid of uniform glass towers |
| **Server Core** | Servers / DMZ | Industrial monoliths inside glowing force-fields |
| **The Outlands** | External (WAN / Internet) addresses | A chaotic, fast-changing ring on the horizon |

- Districts come from a user-supplied CIDR-to-district config, falling back to automatic grouping by private range or /24. 802.1Q VLAN tags, when present, can define districts too.
- In the Outlands, an optional offline GeoIP database places each external host in the compass sector matching its location, so a connection to an East Asian address materialises in the "Eastern Sector".
- **Feasibility:** subnets and VLAN tags are read directly from packet headers. Geolocation needs a local database such as MaxMind GeoLite2, which the user supplies (its licence forbids bundling). Placement is approximate, and no external lookups are made.

### Buildings: hosts

Each active host is a procedurally generated building at a stable position, derived by hashing its address within its district.

- **Height and mass: bandwidth.** A host running a large download or backup becomes a dominant skyscraper; a quiet smart fridge stays low-rise. Height tracks the *recent* rate (bytes/sec with decay on a log scale), so the skyline breathes instead of saturating.
- **Facade windows: active services.** Vertical neon window columns light up per service port in use: a blue column for 80/443 (HTTP/S), a sharp white blinking segment for 22 (SSH), and so on.
  - **Feasibility:** a passive sensor sees ports that are *in use*, not ports that are *open*. A host that answers a TCP SYN with SYN-ACK is marked as offering that service; this never involves scanning.
- **Colour: device class.** For example red for IoT, blue for workstations, green for servers.
  - **Feasibility:** heuristic only, combining the MAC vendor (OUI lookup) with the services a host offers and uses. The inspector labels it as a guess.
- **Shaders: health and status.**
  - *Normal:* sleek holographic blue/cyan.
  - *Degraded* (high latency or retransmissions): "digital static" / VHS tracking artefacts.
  - *Traffic spike:* a white flash sweeps from ground to roof, like an elevator charging to the top; large spikes and scan targets also take a lightning strike.
  - **Feasibility:** latency is measured passively from TCP handshake timing (SYN → SYN-ACK → ACK). Loss is approximated from TCP retransmissions and duplicate sequence numbers. Both are available only for TCP flows the sensor sees from the start.

### Highways and freight: conversations and packets

- **Data highways:** glowing multi-lane roads connect hosts that are actively talking; lane width reflects throughput.
- **Particle freight:** packets drawn as photons or geometric "trains" travelling the highways. To keep the frame rate up, one particle stands for an aggregated batch (see *Performance* below), never one particle per packet.
- **Colour coding:**
  - 🟢 green: web traffic (HTTPS)
  - 🟡 yellow: DNS
  - 🔴 red: cleartext / legacy or unrecognised protocols (Telnet, FTP, plain HTTP auth, unknown ports)
  - **Feasibility:** classification uses port and protocol only. The concept's "malicious payloads" category needs payload inspection, which this project deliberately does not do, so red means "risky or unknown protocol", not "detected malware".
- **Particle speed:** proportional to measured round-trip time where a handshake RTT is known; otherwise a default speed.

## Events: turning anomalies into cinema

Detectors in the Go backend emit `events` alongside flow data, and the frontend plays the matching effect. Thresholds are configurable, and every event appears in an on-screen log with its evidence (source, targets, counts, window).

| Event | Detection (passive heuristic) | Visual |
|---|---|---|
| **Port scan: the Searchlight** | One source sends SYNs (or UDP probes) to many distinct ports or hosts within a short window, mostly without completing handshakes | The scanner's building sweeps a red searchlight cone across the city, touching each probed building |
| **Flood / DDoS: the Meteor Shower** | Packet or connection rate to one destination far above its baseline, from many distinct sources | The sky turns dark red; hostile particles rain onto the target, whose force-field flickers |
| **Host gone dark: the Power Outage** | No packets from a host for a configurable idle timeout | Windows flicker erratically, then the building loses integrity and dissolves into the grid |

**Feasibility:** these are heuristics, not an IDS. The README and UI must say so, link to tools such as Suricata or Zeek for real detection, and design the event schema so external alerts could be fed in later.

## Performance: aggregate and throttle

A busy network produces thousands of packets per second, and drawing one object per packet would freeze the renderer.

- The backend aggregates flows into **100 ms windows** (as today) and sends one message per window.
- The frontend maps each aggregate onto existing objects: one highway with a throughput multiplier instead of 500 lines; particle counts capped and scaled.
- Buildings use instanced meshes, idle hosts are evicted least-recently-used first, and per-frame allocation is avoided.

## Technology choices

- **Backend: Go + gopacket/libpcap.** Chosen over Python/scapy for throughput on gigabit links without dropping packets, and over C++ for safety and a single static binary.
- **Frontend: three.js (WebGL)** in the browser, with post-processing bloom for the neon look. No install is needed beyond the binary, and moving to WebGPU later is possible. Babylon.js was the comparable 3D option; PixiJS or p5.js would suit a 2D view such as the radar.
- **Out of scope:** Unreal Engine 5 / Unity "war room" desktop builds. They would offer higher fidelity but defeat the single-binary, browser-based design.

## Alternative views (future)

The original concept exploration produced other metaphors that use the same data stream. The frontend keeps data handling separate from rendering so these could become extra views later:

| View | Metaphor | Mapping |
|---|---|---|
| **Star Cluster / Node Graph** | Glowing spheres joined by laser beams | Node size/brightness = traffic; beams = connections; photons = packets |
| **Radial Sonar / Radar** | Spinning tactical radar | Distance from centre = latency or internal vs external; blips with fading trails on transmit |
| **Particle Vortex** | A swirling whirlpool | Core = gateway router; orbiting clients, where heavy talkers orbit faster with denser particle streams |

The Cyber-City remains the primary view and the focus of the roadmap.
