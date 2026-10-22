# 🏙️ CyberCity-3D: Real-Time Promiscuous Network Visualizer

CyberCity-3D is a real-time network traffic visualizer that transforms live network packet data captured from a promiscuous network interface into a procedurally generated, cinematic 3D metropolis.

> **Status: early prototype.** Today it draws a wireframe building per host whose height grows with traffic. See [ROADMAP.md](ROADMAP.md) for the plan and [docs/DESIGN.md](docs/DESIGN.md) for the full concept.
>
> **Visibility:** on a switched network, promiscuous mode only sees broadcast/multicast traffic and this host's own traffic. To see every host, capture from a SPAN/mirror port or a network TAP.
>
> **Security:** the current sniffer's WebSocket is unauthenticated and listens on all interfaces. Run it only on a trusted network until Milestone 1 lands.

## 🚀 Getting Started

### 1. Run the Backend Sniffer
Requires Go and libpcap developer headers.
```bash
cd sniffer
go mod tidy
sudo go run main.go
```

### 2. Run the Frontend
```bash
cd frontend
npm install
npm run dev
```

### 3. Mock Traffic Generator (Optional)
If you don't want to run as root or want to test immediately:
```bash
cd tools/generator
npm install
node mock-traffic-flooder.js
```
