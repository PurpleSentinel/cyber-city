import { describe, expect, it } from "vitest";
import { checkSession } from "../src/net/session";

const respond = (status: number): typeof fetch => (async () => new Response(null, { status })) as typeof fetch;

describe("checkSession", () => {
  it("maps 204 to valid and 401 to invalid", async () => {
    expect(await checkSession(respond(204))).toBe("valid");
    expect(await checkSession(respond(401))).toBe("invalid");
  });

  it("treats other statuses and network errors as unreachable", async () => {
    expect(await checkSession(respond(503))).toBe("unreachable");
    const failing = (async () => {
      throw new TypeError("network");
    }) as typeof fetch;
    expect(await checkSession(failing)).toBe("unreachable");
  });
});
