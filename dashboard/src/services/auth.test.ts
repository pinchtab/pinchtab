import { describe, expect, it, vi } from "vitest";
import {
  AUTH_REQUIRED_EVENT,
  AUTH_STATE_CHANGED_EVENT,
  INSECURE_DASHBOARD_TRANSPORT_WARNING,
  SERVER_UNREACHABLE_EVENT,
  dispatchAuthRequired,
  dispatchAuthStateChanged,
  dispatchServerUnreachable,
  isInsecureDashboardTransport,
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

  it("dispatches the server-unreachable event", () => {
    const handler = vi.fn();
    window.addEventListener(SERVER_UNREACHABLE_EVENT, handler);

    dispatchServerUnreachable();

    expect(handler).toHaveBeenCalledTimes(1);
    window.removeEventListener(SERVER_UNREACHABLE_EVENT, handler);
  });

  it("keeps dashboard URLs same-origin without appending auth state", () => {
    vi.stubGlobal("location", new URL("https://pinchtab.com/dashboard"));

    expect(sameOriginUrl("/api/events?memory=1")).toBe("/api/events?memory=1");

    vi.unstubAllGlobals();
  });

  it("detects insecure LAN dashboard transport", () => {
    vi.stubGlobal("location", new URL("http://192.168.1.50:9867/dashboard"));

    expect(isInsecureDashboardTransport()).toBe(true);
    expect(INSECURE_DASHBOARD_TRANSPORT_WARNING).toContain("insecure HTTP");

    vi.unstubAllGlobals();
  });

  it("does not flag localhost over plain http as insecure LAN transport", () => {
    vi.stubGlobal("location", new URL("http://localhost:9867/dashboard"));

    expect(isInsecureDashboardTransport()).toBe(false);

    vi.unstubAllGlobals();
  });
});
