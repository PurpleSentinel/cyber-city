# Security Policy

CyberCity-3D captures network metadata, so its own security matters. This page covers what is supported, how to report a problem and what the design defends against.

## Supported versions

Until the first tagged release, only the `main` branch is supported. After that, the latest release and `main` receive security fixes.

## Reporting a vulnerability

Please report privately through GitHub: **Security → Report a vulnerability** on this repository. Do not open a public issue for security problems.

Include what you found, how to reproduce it, and the impact you expect. You should get an acknowledgement within a week. A fix or mitigation is coordinated with you before anything is disclosed.

## Threat model (summary)

Full version: [docs/DESIGN.md](docs/DESIGN.md#threat-model-summary).

**What is protected:** metadata about the local network and its users: which hosts talk to which, when, how much, and on which ports. Packet payloads are never captured beyond a 256-byte snap length, and never stored or sent.

**Attackers considered**
- A malicious web page open in the viewer's browser trying to read the feed (cross-site WebSocket hijacking).
- Other machines on the LAN.
- Other local users on the sensor machine.

**Defences**

| Risk | Defence |
|---|---|
| Remote access | Binds `127.0.0.1` by default; any non-loopback bind logs a warning. |
| Cross-site pages | A one-time token in the printed URL is exchanged for an `HttpOnly`, `SameSite=Strict` session cookie; `/ws` and `/api/*` require it. WebSocket upgrades also need an allowed `Origin`. |
| Injected content | Strict Content-Security-Policy, `nosniff`, `no-referrer`; no inline scripts or styles. |
| Privilege | Use `setcap` to grant only capture capabilities, or start as root and the process drops to `nobody` (or `--user`) once the capture and listener are open. |
| Slow or abusive clients | Per-client queues and write deadlines, a 512-byte read limit, ping/pong liveness, a client cap. |
| Recordings and screenshots | `--anonymize` replaces addresses with per-run pseudonyms; MAC addresses are off unless `--show-mac`. |
| Supply chain | Pinned dependencies, Dependabot, `govulncheck` in CI. |

**Out of scope:** a compromised root account on the sensor, and people the operator deliberately gives the link to. The URL token is a bearer secret: anyone who has it can view the feed until the process restarts.

**Not an IDS:** port-scan and flood events are heuristics for visualisation. Use a dedicated tool such as Suricata or Zeek for detection you rely on.
