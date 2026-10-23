# Setup and quick start

This guide takes you from a clean Linux machine to a running city: first replaying the bundled demo capture, which needs no root, then capturing live traffic. For every flag and default, see [CONFIG.md](CONFIG.md). For the security model, see [../SECURITY.md](../SECURITY.md).

Commands in `bash` blocks are run by CI (`tools/ci/readme-smoke.sh`, `tools/ci/live-quickstart.sh`). Blocks marked *example* depend on your machine or network, so CI cannot run them.

## 1. Prerequisites

You need Linux, Go ≥ 1.25, Node 22 (see `.nvmrc`), and the libpcap headers.

*Example: install the toolchain.*

```text
# Fedora
sudo dnf install golang libpcap-devel nodejs make
# Debian / Ubuntu
sudo apt install golang libpcap-dev nodejs npm make
```

Distribution packages can lag behind. If `go version` reports less than 1.25, install Go from <https://go.dev/dl/>. If `node --version` is not 22.x, use a Node version manager such as `nvm use` in the repository.

You also need a browser with **hardware-accelerated WebGL 2**: a current Chrome, Chromium, Edge or Firefox on a machine with a working GPU driver. See [section 6](#6-browser-and-gpu).

## 2. Build

```bash
make setup     # go mod download, npm ci in web/, Playwright Chromium and its system libraries
make build     # vite build, then go build: bin/cybercity with the frontend embedded
bin/cybercity --version
```

`make setup` installs Playwright only for the end-to-end tests. To build without it, run `go mod download` and `cd web && npm ci` instead.

The result is a single binary: `bin/cybercity` serves the page itself, so there is nothing else to deploy.

## 3. First run: the demo capture (no root)

```bash
bin/cybercity --read testdata/pcap/demo.pcap --loop
```

It prints a line like this:

```text
INFO ready url="http://127.0.0.1:8080/auth?token=…"
```

Open that URL. The token logs your browser in (it is swapped for a session cookie) and is removed from the address bar. The token stays valid until the process stops, so keep it private. Anyone with the link can view the feed.

What to expect from the demo, which is 2 minutes of synthetic traffic, looped:
- Two automatic districts on lit plots, `LAN 10.0.1.0/24` and `LAN 10.0.9.0/24` (name them with a districts file, [section 5](#5-configuration)). External hosts sit in the **Outlands** ring.
- Buildings grow with their traffic. Highways and freight particles show who talks to whom.
- About 60 s into each loop, a SYN scan from `10.0.1.66` triggers a **searchlight**, and the event log (bottom right) records the `port_scan`.
- Click a building to inspect it, and press `?` for the keys.

To look at the raw protocol messages instead of the page:

```bash
bin/cybercity --read testdata/pcap/demo.pcap --dump-json - > demo.jsonl
head -c 300 demo.jsonl
```

Other synthetic captures live in `testdata/pcap/` (see [TESTING.md](TESTING.md#fixtures)). For example, `flood.pcap` shows the meteor shower about 45 s in.

## 4. Live capture

### Permissions

Capturing packets needs `CAP_NET_RAW` and `CAP_NET_ADMIN`. The recommended way is to grant them to the binary once and then run it as yourself:

```bash
make setcap          # sudo setcap cap_net_raw,cap_net_admin+eip bin/cybercity
bin/cybercity
```

Run `make setcap` again after each `make build`, because rebuilding replaces the file and loses the capabilities.

*Example: the alternative is to start as root.* The binary opens the capture, then drops to `nobody` (or `--user NAME`) before serving anything:

```text
sudo bin/cybercity
```

Never run it as root without that drop. The server refuses to continue if it cannot shed root.

### Choosing what to capture

```bash
bin/cybercity --list-ifaces
```

By default it captures on the interface that holds the default route. Pick another one with `--iface`. You can narrow the traffic with a BPF filter, and `--no-promisc` limits the capture to this machine's own traffic plus broadcast and multicast:

```bash
bin/cybercity --iface lo --bpf "tcp or udp" --no-promisc
```

### Seeing the whole network

A switch delivers to your port only traffic addressed to your machine, plus broadcast and multicast. Promiscuous mode cannot change that. If the city shows little more than your own machine, you need a copy of the traffic:

- **SPAN / port mirroring:** configure the switch to mirror the uplink or a VLAN to the port the sensor is plugged into. This is managed switches only; look for "port mirroring" in the switch's web UI.
- **Network TAP:** a passive device inline on a link, which copies both directions to a monitor port.
- **This machine is the router or gateway:** capture on its LAN interface, and you see everything that crosses it.

Then run with `--iface` set to the interface that receives the copy.

## 5. Configuration

Everything can also go in a YAML file. The example file lists every default:

```bash
cp config/cybercity.example.yaml cybercity.yaml
bin/cybercity --config cybercity.yaml --read testdata/pcap/demo.pcap --dump-json - > /dev/null
```

Flags override the file. The keys are documented in [CONFIG.md](CONFIG.md).

**Districts.** By default, each local subnet becomes an automatic district. To name them, or to group subnets and VLANs, write a districts file:

```bash
bin/cybercity --districts config/districts.example.yaml --read testdata/pcap/vlan.pcap --dump-json - > /dev/null
```

**GeoIP (optional).** Place external hosts by compass bearing from your location. You need your own GeoLite2-City database: free from MaxMind with an account, and it is not bundled. Set your location in the config file:

*Example: the GeoIP part of `cybercity.yaml`.*

```text
geoip: /path/to/GeoLite2-City.mmdb
geo:
  home: [51.5, -0.12]   # your latitude, longitude
```

The lookup is local: cybercity never makes network calls at runtime.

## 6. Browser and GPU

The city is rendered by three.js on WebGL 2. The HUD shows which renderer the browser picked:

- `Renderer: <your GPU>`: good.
- `⚠ Software rendering — enable GPU acceleration`: the browser fell back to a CPU renderer (llvmpipe or SwiftShader). The page switches to low quality automatically, but it will still be slow.

*Example: turning on hardware acceleration.*
- **Chrome / Edge:** Settings → System → "Use graphics acceleration when available", then restart. `chrome://gpu` shows the result.
- **Firefox:** Settings → General → Performance: untick "Use recommended performance settings" and tick "Use hardware acceleration when available". `about:support` shows the result.
- **Linux:** make sure the GPU driver is installed (Mesa for Intel and AMD, or the vendor driver).

Press `L` to toggle low quality, which turns off bloom and shadows and drops to pixel ratio 1. You can also force a mode with `&quality=low` or `&quality=high` in the page URL (after `/?`).

## 7. Viewing from another machine

The server listens on `127.0.0.1` only. The safe way to watch a remote sensor is an SSH tunnel that **uses the same port on both ends**:

*Example: on your workstation.*

```text
ssh -L 8080:127.0.0.1:8080 sensor-host
```

Then open the URL that the sensor printed, unchanged. It must be `127.0.0.1`, not `localhost`: the WebSocket only accepts its own origin, and a different host name or port is refused. If you must use a different local port, start the server with `--allow-origin http://127.0.0.1:<local-port>`.

`--listen 0.0.0.0:8080` exposes the page to your whole network. It logs a warning, because anyone who obtains the link can see your traffic metadata. Prefer the tunnel.

## 8. Privacy for screenshots and recordings

- `--anonymize` replaces every host id with a per-run pseudonym (`anon-…`) and sends `ip: null`.
- MAC addresses are never sent unless you pass `--show-mac`.
- Only packet headers are decoded. At most `--snaplen` (256) bytes per packet are captured, and payloads are never stored or sent.

```bash
bin/cybercity --anonymize --read testdata/pcap/demo.pcap --dump-json - > anon.jsonl
```

## 9. Troubleshooting

| Symptom | Cause and fix |
|---|---|
| Page says **Invalid or expired link** | The server restarted (new token), or the session cookie is missing. Open the URL it printed most recently. |
| `permission denied` / `operation not permitted` opening the interface | No capture rights. Run `make setcap` (again after every rebuild) or start with `sudo`. |
| The city shows only this machine | Switched network: see [Seeing the whole network](#seeing-the-whole-network). |
| **Kernel drops** turns red in the HUD | The capture cannot keep up. Narrow it with `--bpf` (for example `"not port 22"`), or use a less busy interface. |
| `address already in use` | Port 8080 is taken. Use `--listen 127.0.0.1:0` to pick a free port; `--url-file path` writes the URL to a file you can read. |
| Behind a tunnel or proxy, the status cycles through **Connecting…** / **Reconnecting** | Origin mismatch. Open the exact printed URL, or add `--allow-origin`. See [section 7](#7-viewing-from-another-machine). |
| Very low frame rate | Software rendering: see [section 6](#6-browser-and-gpu). Press `L` for low quality. |

Events (searchlight, meteor shower, power outage) come from simple thresholds over packet metadata ([CONFIG.md](CONFIG.md#event-detectors)). They are **heuristic detections, not an IDS**: expect both false positives and misses.

## 10. Cleanup

*Example: remove the capabilities and build output.*

```text
sudo setcap -r bin/cybercity   # drop the capture capabilities
make clean                     # remove bin/ and the built frontend
```
