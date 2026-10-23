# 🏙️ CyberCity-3D: Real-Time Promiscuous Network Visualizer

CyberCity-3D is a real-time network traffic visualizer that transforms live network packet data captured from a promiscuous network interface into a procedurally generated, cinematic 3D metropolis.

> **Status: early prototype.** Today it draws a wireframe building per host whose height grows with traffic. See [ROADMAP.md](ROADMAP.md) for the plan and [docs/DESIGN.md](docs/DESIGN.md) for the full concept.
>
> **Visibility:** on a switched network, promiscuous mode only sees broadcast/multicast traffic and this host's own traffic. To see every host, capture from a SPAN/mirror port or a network TAP.
>
> **Security:** safe by default: loopback-only, token-and-cookie login, origin checks, no packet payloads, and root dropped after the capture opens. See [SECURITY.md](SECURITY.md).

## 🚀 Getting Started

Requires Go ≥ 1.25, Node 22 and libpcap headers (Fedora: `sudo dnf install golang libpcap-devel nodejs`; Debian/Ubuntu: `sudo apt install golang libpcap-dev nodejs npm`).

```bash
make setup     # Go modules, web dependencies, Playwright browser
make build     # builds the frontend and embeds it in bin/cybercity
```

Live capture needs packet-capture rights. Pick one:

```bash
make setcap && bin/cybercity   # recommended: grant only CAP_NET_RAW/CAP_NET_ADMIN, run as yourself
sudo bin/cybercity             # or start as root: it drops to `nobody` (or --user NAME) once the capture is open
```

Then open the URL it prints (`http://127.0.0.1:8080/auth?token=…`): the one-time token logs your browser in, so other web pages and users cannot read the feed. One process captures traffic and serves the page.

**No root needed:** replay the bundled synthetic capture instead of capturing live:

```bash
bin/cybercity --read testdata/pcap/demo.pcap --loop
bin/cybercity --read testdata/pcap/demo.pcap --dump-json -   # print the protocol messages
```

See [docs/TASKS.md](docs/TASKS.md) for progress. Contributors: start with [AGENTS.md](AGENTS.md).
