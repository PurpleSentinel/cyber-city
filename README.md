# 🏙️ CyberCity-3D: Real-Time Promiscuous Network Visualizer

CyberCity-3D is a real-time network traffic visualizer that transforms live network packet data captured from a promiscuous network interface into a procedurally generated, cinematic 3D metropolis.

> **Status: early prototype.** Today it draws a wireframe building per host whose height grows with traffic. See [ROADMAP.md](ROADMAP.md) for the plan and [docs/DESIGN.md](docs/DESIGN.md) for the full concept.
>
> **Visibility:** on a switched network, promiscuous mode only sees broadcast/multicast traffic and this host's own traffic. To see every host, capture from a SPAN/mirror port or a network TAP.
>
> **Security:** the current sniffer's WebSocket is unauthenticated and listens on all interfaces. Run it only on a trusted network until Milestone 1 lands.

## 🚀 Getting Started

Requires Go ≥ 1.25, Node 22 and libpcap headers (Fedora: `sudo dnf install golang libpcap-devel nodejs`; Debian/Ubuntu: `sudo apt install golang libpcap-dev nodejs npm`).

```bash
make setup     # Go modules, web dependencies, Playwright browser
make build     # builds the frontend and embeds it in bin/cybercity
sudo bin/cybercity
```

Then open http://127.0.0.1:8080. One process captures traffic and serves the page.

**No root needed:** replay the bundled synthetic capture instead of capturing live:

```bash
bin/cybercity --read testdata/pcap/demo.pcap --loop
bin/cybercity --read testdata/pcap/demo.pcap --dump-json -   # print the protocol messages
```

See [docs/TASKS.md](docs/TASKS.md) for progress. Contributors: start with [AGENTS.md](AGENTS.md).
