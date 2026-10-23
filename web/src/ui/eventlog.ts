// Event log (docs/VISUAL_SPEC.md "HUD and interaction"): the last 50 events,
// newest first. Formatting is pure; EventLog only writes DOM text.
import { HUD } from "../config";
import type { ProtocolEvent } from "../protocol";

export const EVENT_LOG_FOOTER = "Heuristic detections — not an IDS";

const KIND_LABELS: Record<string, string> = {
  port_scan: "Port scan",
  flood: "Flood",
  host_idle: "Host idle",
};

const pad = (n: number): string => String(n).padStart(2, "0");

/** Local wall-clock time of an event, HH:MM:SS. */
export function formatTime(ts: number): string {
  const d = new Date(ts);
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

/** "src → t1, t2, t3 +N"; a flood without a single source reads "many → …". */
export function formatParties(e: ProtocolEvent): string {
  const src = e.src ?? "many";
  const targets = e.targets.length > 0 ? e.targets : e.dst ? [e.dst] : [];
  if (targets.length === 0) return src;
  const shown = targets.slice(0, 3).join(", ");
  const more = targets.length > 3 ? ` +${targets.length - 3}` : "";
  return `${src} → ${shown}${more}`;
}

/** Short evidence summary per event kind. */
export function formatEvidence(e: ProtocolEvent): string {
  const v = e.evidence;
  const pct = (x: number | undefined): string => `${Math.round((x ?? 0) * 100)}% incomplete`;
  switch (e.kind) {
    case "port_scan":
      return (v.distinct_hosts ?? 0) > 1
        ? `${v.distinct_hosts} hosts · ${pct(v.incomplete_ratio)}`
        : `${v.distinct_ports} ports · ${pct(v.incomplete_ratio)}`;
    case "flood":
      return `${Math.round(v.pps ?? 0)} pkt/s (baseline ${Math.round(v.baseline_pps ?? 0)}) · ${v.distinct_sources} sources`;
    case "host_idle":
      return `idle ${Math.round(v.idle_s ?? 0)} s`;
    default:
      return Object.entries(v)
        .map(([k, x]) => `${k}=${x}`)
        .join(" ");
  }
}

export interface LogEntry {
  id: string;
  severity: ProtocolEvent["severity"];
  time: string;
  kind: string;
  parties: string;
  evidence: string;
}

/** Rows for the newest HUD.eventLogSize events (input is newest first). */
export function logEntries(events: readonly ProtocolEvent[]): LogEntry[] {
  return events.slice(0, HUD.eventLogSize).map((e) => ({
    id: e.id,
    severity: e.severity,
    time: formatTime(e.ts),
    kind: KIND_LABELS[e.kind] ?? e.kind,
    parties: formatParties(e),
    evidence: formatEvidence(e),
  }));
}

export class EventLog {
  private readonly list: HTMLOListElement;

  constructor(doc: Document = document) {
    const panel = doc.createElement("aside");
    panel.id = "eventlog";
    const title = doc.createElement("h2");
    title.textContent = "Events";
    this.list = doc.createElement("ol");
    const footer = doc.createElement("p");
    footer.className = "footer";
    footer.textContent = EVENT_LOG_FOOTER;
    panel.append(title, this.list, footer);
    doc.body.append(panel);
  }

  render(events: readonly ProtocolEvent[]): void {
    const doc = this.list.ownerDocument;
    this.list.replaceChildren(
      ...logEntries(events).map((r) => {
        const li = doc.createElement("li");
        li.className = r.severity;
        li.dataset.event = r.id;
        const cells: [string, string][] = [
          ["time", r.time],
          ["kind", r.kind],
          ["parties", r.parties],
          ["evidence", r.evidence],
        ];
        for (const [cls, text] of cells) {
          const span = doc.createElement("span");
          span.className = cls;
          span.textContent = text;
          li.append(span);
        }
        return li;
      }),
    );
  }
}
