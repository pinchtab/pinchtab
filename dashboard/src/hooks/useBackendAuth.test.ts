import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as api from "../services/api";
import { AUTH_REQUIRED_EVENT } from "../services/auth";
import {
  AUTH_RETRY_DELAYS_MS,
  backendAuthReducer,
  initialBackendAuthState,
  nextRetryDelayMs,
  useBackendAuth,
  type BackendAuthState,
} from "./useBackendAuth";

const unreachable = (retryCount: number): BackendAuthState => ({
  mode: "unreachable",
  authProtected: false,
  retryCount,
});

describe("backendAuthReducer", () => {
  it("backs off through every retry delay, then stops", () => {
    let state = backendAuthReducer(initialBackendAuthState, {
      type: "probeFailed",
    });
    const delays: (number | null)[] = [];
    for (let i = 0; i <= AUTH_RETRY_DELAYS_MS.length; i++) {
      delays.push(nextRetryDelayMs(state.retryCount));
      state = backendAuthReducer(
        backendAuthReducer(state, { type: "retryElapsed" }),
        { type: "probeFailed" },
      );
    }
    expect(delays).toEqual([...AUTH_RETRY_DELAYS_MS, null]);
  });

  it("retry now restarts the backoff from the first delay", () => {
    const state = backendAuthReducer(unreachable(5), { type: "retryNow" });
    expect(state).toEqual({ ...unreachable(0), mode: "probing" });
  });

  it("a successful probe resets the backoff", () => {
    const state = backendAuthReducer(
      { ...unreachable(3), mode: "probing" },
      { type: "probeSucceeded", mode: "required" },
    );
    expect(state).toEqual({
      mode: "required",
      authProtected: true,
      retryCount: 0,
    });
  });

  it("the auth-required event demands login and marks auth protected", () => {
    const state = backendAuthReducer(
      { mode: "open", authProtected: false, retryCount: 2 },
      { type: "authRequired" },
    );
    expect(state).toEqual({
      mode: "required",
      authProtected: true,
      retryCount: 0,
    });
  });
});

describe("useBackendAuth", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it("switches to required when the auth-required event fires", async () => {
    vi.spyOn(api, "probeBackendAuth").mockResolvedValue({ mode: "open" });
    const { result } = renderHook(() => useBackendAuth());
    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.mode).toBe("open");

    act(() => {
      window.dispatchEvent(new Event(AUTH_REQUIRED_EVENT));
    });
    expect(result.current.mode).toBe("required");
    expect(result.current.authProtected).toBe(true);
  });

  it("reprobes after the first backoff delay", async () => {
    vi.useFakeTimers();
    const probe = vi
      .spyOn(api, "probeBackendAuth")
      .mockRejectedValueOnce(new Error("down"))
      .mockResolvedValue({ mode: "open" });
    vi.spyOn(console, "error").mockImplementation(() => {});
    const { result } = renderHook(() => useBackendAuth());
    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.mode).toBe("unreachable");
    expect(result.current.retryDelay).toBe(AUTH_RETRY_DELAYS_MS[0]);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(AUTH_RETRY_DELAYS_MS[0]);
    });
    expect(probe).toHaveBeenCalledTimes(2);
    expect(result.current.mode).toBe("open");
    expect(result.current.retryCount).toBe(0);
  });
});
