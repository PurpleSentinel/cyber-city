// Inspector panel and hover tooltip (docs/VISUAL_SPEC.md "HUD and
// interaction"). Rows are built by a pure function; DOM writes use textContent.
import type { Host } from "../protocol";
import { formatRate } from "./hud";

export type Row = [label: string, value: string];

const time = (ms: number): string => new Date(ms).toISOString().slice(11, 19);

/** Inspector rows for a host. */
export function inspectorRows(h: Host, districtName: string, rateBps: number): Row[] {
  const services =
    h.services.length === 0
      ? "—"
      : h.services.map((s) => `${s.port}/${s.proto} ${s.category}${s.confirmed ? "" : " (unconfirmed)"}`).join(", ");
  return [
    ["Host", h.ip ?? h.id],
    ["Vendor", h.vendor ?? "—"],
    ["Class", `${h.device_class} (heuristic)`],
    ["District", districtName],
    ["Services", services],
    ["Traffic", formatRate(rateBps)],
    ["RTT", h.rtt_ms === null ? "—" : `${h.rtt_ms.toFixed(1)} ms`],
    ["Retransmits", `${(h.retrans_rate * 100).toFixed(1)}%`],
    ["First seen", time(h.first_seen)],
    ["Last seen", time(h.last_seen)],
  ];
}

/** Tooltip text: id and vendor. */
export function tooltipText(h: Host): string {
  return h.vendor ? `${h.id} · ${h.vendor}` : h.id;
}

export class InspectorView {
  private readonly panel: HTMLElement;
  private readonly tip: HTMLElement;
  selected: string | null = null;

  constructor(doc: Document = document) {
    this.panel = doc.createElement("aside");
    this.panel.id = "inspector";
    this.panel.hidden = true;
    this.tip = doc.createElement("div");
    this.tip.id = "tooltip";
    this.tip.hidden = true;
    doc.body.append(this.panel, this.tip);
  }

  show(id: string, rows: readonly Row[]): void {
    this.selected = id;
    const doc = this.panel.ownerDocument;
    const title = doc.createElement("h2");
    title.textContent = "Inspector";
    const dl = doc.createElement("dl");
    for (const [label, value] of rows) {
      const dt = doc.createElement("dt");
      dt.textContent = label;
      const dd = doc.createElement("dd");
      dd.textContent = value;
      dl.append(dt, dd);
    }
    this.panel.replaceChildren(title, dl);
    this.panel.dataset.host = id;
    this.panel.hidden = false;
  }

  clear(): void {
    this.selected = null;
    this.panel.hidden = true;
    delete this.panel.dataset.host;
  }

  hover(text: string | null, x = 0, y = 0): void {
    if (text === null) {
      this.tip.hidden = true;
      return;
    }
    this.tip.textContent = text;
    this.tip.style.transform = `translate(${x + 14}px, ${y + 14}px)`; // CSSOM, not an inline style attribute
    this.tip.hidden = false;
  }
}
