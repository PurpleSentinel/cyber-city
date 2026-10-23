import { describe, expect, it } from "vitest";
import { parseBatch } from "../src/prototype";

const flow = {
  timestamp: 1,
  src_ip: "10.0.1.10",
  src_mac: "",
  src_is_local: true,
  dst_ip: "203.0.113.10",
  dst_mac: "",
  dst_is_local: false,
  protocol: "TCP",
  port: 443,
  packet_count: 3,
  byte_count: 1800,
  flags: ["ACK"],
};

describe("parseBatch", () => {
  it("returns valid flows", () => {
    expect(parseBatch(JSON.stringify([flow]))).toEqual([flow]);
  });

  it("drops malformed records and non-array frames", () => {
    expect(parseBatch(JSON.stringify([flow, { src_ip: 1 }]))).toHaveLength(1);
    expect(parseBatch(JSON.stringify({ not: "an array" }))).toEqual([]);
    expect(parseBatch("{bad json")).toEqual([]);
  });
});
