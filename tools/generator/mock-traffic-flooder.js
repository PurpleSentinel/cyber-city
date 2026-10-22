const WebSocket = require('ws');

const wss = new WebSocket.Server({ port: 8080 });
console.log("Mock Traffic Generator running WS Server on port 8080...");

function generateMockBatch() {
    const protocols = ["HTTPS", "DNS", "SSH", "TCP"];
    const localIPs = ["192.168.1.10", "192.168.1.15", "192.168.1.44", "192.168.1.101"];
    const externalIPs = ["8.8.8.8", "142.250.190.46", "23.221.22.102", "52.114.77.33"];
    
    const batch = [];
    const count = Math.floor(Math.random() * 5) + 1;
    
    for (let i = 0; i < count; i++) {
        const isOutbound = Math.random() > 0.5;
        batch.push({
            timestamp: Date.now(),
            src_ip: isOutbound ? localIPs[Math.floor(Math.random() * localIPs.length)] : externalIPs[Math.floor(Math.random() * externalIPs.length)],
            src_mac: "00:1A:2B:3C:4D:5E",
            src_is_local: isOutbound,
            dst_ip: isOutbound ? externalIPs[Math.floor(Math.random() * externalIPs.length)] : localIPs[Math.floor(Math.random() * localIPs.length)],
            dst_mac: "00:11:22:33:44:55",
            dst_is_local: !isOutbound,
            protocol: protocols[Math.floor(Math.random() * protocols.length)],
            port: Math.random() > 0.5 ? 443 : 80,
            packet_count: Math.floor(Math.random() * 10) + 1,
            byte_count: Math.floor(Math.random() * 15000) + 500,
            flags: ["ACK"]
        });
    }
    return JSON.stringify(batch);
}

wss.on('connection', (ws) => {
    console.log("Visualizer connected to mock stream.");
    const interval = setInterval(() => {
        if (ws.readyState === WebSocket.OPEN) {
            ws.send(generateMockBatch());
        }
    }, 100);

    ws.on('close', () => clearInterval(interval));
});
