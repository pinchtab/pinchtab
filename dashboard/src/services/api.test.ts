import { beforeEach, describe, expect, it, vi } from "vitest";
import { createProfile, fetchProfiles, probeBackendAuth } from "./api";

describe("api request headers", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("tags dashboard GET requests with the dashboard source header", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [],
    });
    vi.stubGlobal("fetch", fetchMock);

    await fetchProfiles();

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(new Headers(init.headers).get("X-PinchTab-Source")).toBe(
      "dashboard",
    );
    expect(init.credentials).toBe("same-origin");
  });

  it("preserves request headers while tagging dashboard POST requests", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ status: "ok", id: "prof_123", name: "demo" }),
    });
    vi.stubGlobal("fetch", fetchMock);

    await createProfile({ name: "demo" });

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = new Headers(init.headers);
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(headers.get("X-PinchTab-Source")).toBe("dashboard");
  });

  it("tags auth probe requests too", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        version: "test",
        uptime: 1,
        profiles: 0,
        instances: 0,
        agents: 0,
        authRequired: true,
      }),
    });
    vi.stubGlobal("fetch", fetchMock);

    await probeBackendAuth();

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(new Headers(init.headers).get("X-PinchTab-Source")).toBe(
      "dashboard",
    );
  });
});
