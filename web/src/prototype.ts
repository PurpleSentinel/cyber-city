// Prototype wire format, still sent by the backend until T0.8: each WebSocket frame is a JSON array of per-window flow
// records. The v1 schema (docs/PROTOCOL.md) is in protocol.ts.

export interface PrototypeFlow {
  timestamp: number;
  src_ip: string;
  src_mac: string;
  src_is_local: boolean;
  dst_ip: string;
  dst_mac: string;
  dst_is_local: boolean;
  protocol: string;
  port: number;
  packet_count: number;
  byte_count: number;
  flags: string[] | null;
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

export function isPrototypeFlow(v: unknown): v is PrototypeFlow {
  if (!isRecord(v)) return false;
  return (
    typeof v.src_ip === "string" &&
    typeof v.dst_ip === "string" &&
    typeof v.src_is_local === "boolean" &&
    typeof v.dst_is_local === "boolean" &&
    typeof v.byte_count === "number"
  );
}

// parseBatch returns the valid flows in a frame and drops anything malformed,
// so one bad record cannot stop the render loop.
export function parseBatch(raw: string): PrototypeFlow[] {
  let data: unknown;
  try {
    data = JSON.parse(raw);
  } catch {
    return [];
  }
  return Array.isArray(data) ? data.filter(isPrototypeFlow) : [];
}
