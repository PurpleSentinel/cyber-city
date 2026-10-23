// SPDX-License-Identifier: MIT
// Session check before opening the WebSocket (docs/PROTOCOL.md "Client behaviour").

export type SessionState = "valid" | "invalid" | "unreachable";

/** Asks the server whether this browser holds a valid session cookie. */
export async function checkSession(fetchFn: typeof fetch = fetch): Promise<SessionState> {
  try {
    const res = await fetchFn("/api/session", { credentials: "same-origin", cache: "no-store" });
    if (res.status === 204) return "valid";
    if (res.status === 401) return "invalid";
    return "unreachable";
  } catch {
    return "unreachable";
  }
}

export const INVALID_LINK_MESSAGE = "Invalid or expired link — reopen the URL printed by cybercity";
