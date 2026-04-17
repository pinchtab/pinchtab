import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createProfile,
  fetchProfiles,
  handleRealtimeAuthFailure,
  launchInstance,
  probeBackendAuth,
  resetRealtimeAuthProbeStateForTests,
} from "./api";
import { SERVER_UNREACHABLE_EVENT } from "./auth";

describe("api request headers", () => {
  beforeEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    resetRealtimeAuthProbeStateForTests();
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

  it("starts instances through the canonical start endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        id: "inst_default",
        profileId: "default",
        profileName: "default",
        port: "9868",
        headless: true,
        status: "starting",
        startTime: "2026-03-06T10:00:00Z",
        attached: false,
      }),
    });
    vi.stubGlobal("fetch", fetchMock);

    await launchInstance({ profileId: "default", mode: "headed" });

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/instances/start");
    expect(new Headers(init.headers).get("X-PinchTab-Source")).toBe(
      "dashboard",
    );
  });

  it("dispatches a server-unreachable event when requests lose transport", async () => {
    const handler = vi.fn();
    const fetchMock = vi.fn().mockRejectedValue(new TypeError("NetworkError"));
    vi.stubGlobal("fetch", fetchMock);
    window.addEventListener(SERVER_UNREACHABLE_EVENT, handler);

    await expect(fetchProfiles()).rejects.toThrow("NetworkError");

    expect(handler).toHaveBeenCalledTimes(1);
    window.removeEventListener(SERVER_UNREACHABLE_EVENT, handler);
  });

  it("dispatches a server-unreachable event when realtime auth probing fails", async () => {
    const handler = vi.fn();
    const fetchMock = vi.fn().mockRejectedValue(new TypeError("NetworkError"));
    vi.stubGlobal("fetch", fetchMock);
    window.addEventListener(SERVER_UNREACHABLE_EVENT, handler);

    await handleRealtimeAuthFailure();

    expect(handler).toHaveBeenCalledTimes(1);
    window.removeEventListener(SERVER_UNREACHABLE_EVENT, handler);
  });

  it("deduplicates concurrent realtime auth probes", async () => {
    let resolveFetch:
      | ((value: {
          ok: boolean;
          json: () => Promise<{
            version: string;
            uptime: number;
            profiles: number;
            instances: number;
            agents: number;
            authRequired: boolean;
          }>;
        }) => void)
      | undefined;
    const fetchMock = vi.fn().mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveFetch = resolve;
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const firstProbe = handleRealtimeAuthFailure();
    const secondProbe = handleRealtimeAuthFailure();

    expect(fetchMock).toHaveBeenCalledTimes(1);

    resolveFetch?.({
      ok: true,
      json: async () => ({
        version: "test",
        uptime: 1,
        profiles: 0,
        instances: 0,
        agents: 0,
        authRequired: false,
      }),
    });

    await Promise.all([firstProbe, secondProbe]);
  });

  it("throttles realtime auth probes inside the cooldown window", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-04-15T00:00:00Z"));
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        version: "test",
        uptime: 1,
        profiles: 0,
        instances: 0,
        agents: 0,
        authRequired: false,
      }),
    });
    vi.stubGlobal("fetch", fetchMock);

    await handleRealtimeAuthFailure();
    await handleRealtimeAuthFailure();

    expect(fetchMock).toHaveBeenCalledTimes(1);

    vi.setSystemTime(Date.now() + 3001);
    await handleRealtimeAuthFailure();

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
