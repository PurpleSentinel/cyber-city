# 🏙️ CyberCity-3D: Real-Time Promiscuous Network Visualizer

CyberCity-3D is a real-time network traffic visualizer that transforms live network packet data captured from a promiscuous network interface into a procedurally generated, cinematic 3D metropolis.

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
