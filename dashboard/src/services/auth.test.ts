import { describe, expect, it, vi } from "vitest";
import {
  AUTH_REQUIRED_EVENT,
  AUTH_STATE_CHANGED_EVENT,
  credentialUsername,
  dispatchAuthRequired,
  dispatchAuthStateChanged,
  sameOriginUrl,
} from "./auth";

describe("auth helpers", () => {
  it("dispatches the auth-required event with a reason", () => {
    const handler = vi.fn();
    window.addEventListener(AUTH_REQUIRED_EVENT, handler);

    dispatchAuthRequired("missing_token");

    expect(handler).toHaveBeenCalledTimes(1);
    const event = handler.mock.calls[0]?.[0] as CustomEvent<{ reason: string }>;
    expect(event.detail.reason).toBe("missing_token");
    window.removeEventListener(AUTH_REQUIRED_EVENT, handler);
  });

  it("dispatches the auth-state-changed event", () => {
    const handler = vi.fn();
    window.addEventListener(AUTH_STATE_CHANGED_EVENT, handler);

    dispatchAuthStateChanged();

    expect(handler).toHaveBeenCalledTimes(1);
    window.removeEventListener(AUTH_STATE_CHANGED_EVENT, handler);
  });

  it("keeps dashboard URLs same-origin without appending auth state", () => {
    vi.stubGlobal("location", new URL("https://pinchtab.com/dashboard"));

    expect(sameOriginUrl("/api/events?memory=1")).toBe("/api/events?memory=1");

    vi.unstubAllGlobals();
  });

  it("builds a stable credential username for password managers", () => {
    vi.stubGlobal("location", new URL("https://pinchtab.example/dashboard"));

    expect(credentialUsername()).toBe("pinchtab@pinchtab.example");

    vi.unstubAllGlobals();
  });
});
