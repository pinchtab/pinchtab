import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useAsyncResource, type AsyncFetcher } from "./useAsyncResource";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

async function flush() {
  await act(async () => {
    await Promise.resolve();
  });
}

describe("useAsyncResource", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("loads data and clears loading", async () => {
    const pending = deferred<string>();
    const { result } = renderHook(() =>
      useAsyncResource(() => pending.promise, [], { initial: "" }),
    );
    expect(result.current.loading).toBe(true);

    pending.resolve("ready");
    await flush();

    expect(result.current).toMatchObject({
      data: "ready",
      loading: false,
      error: "",
    });
  });

  it("aborts the in-flight request on unmount and ignores its result", async () => {
    const pending = deferred<string>();
    let signal: AbortSignal | undefined;
    const fetcher: AsyncFetcher<string> = (s) => {
      signal = s;
      return pending.promise;
    };
    const { result, unmount } = renderHook(() =>
      useAsyncResource(fetcher, [], { initial: "initial" }),
    );

    unmount();
    expect(signal?.aborted).toBe(true);

    pending.resolve("late");
    await flush();
    expect(result.current.data).toBe("initial");
  });

  it("drops a stale response that resolves after deps change", async () => {
    const requests = new Map<string, ReturnType<typeof deferred<string>>>();
    const { result, rerender } = renderHook(
      ({ id }) =>
        useAsyncResource(
          () => {
            const request = deferred<string>();
            requests.set(id, request);
            return request.promise;
          },
          [id],
          { initial: "" },
        ),
      { initialProps: { id: "a" } },
    );

    rerender({ id: "b" });
    requests.get("b")!.resolve("from-b");
    await flush();
    requests.get("a")!.resolve("from-a");
    await flush();

    expect(result.current.data).toBe("from-b");
  });

  it("polls at pollMs and stops polling after unmount", async () => {
    vi.useFakeTimers();
    let calls = 0;
    const { result, unmount } = renderHook(() =>
      useAsyncResource(() => Promise.resolve(++calls), [], {
        initial: 0,
        pollMs: 1000,
      }),
    );
    await flush();
    expect(result.current.data).toBe(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(calls).toBe(3);
    expect(result.current.data).toBe(3);

    unmount();
    await vi.advanceTimersByTimeAsync(5000);
    expect(calls).toBe(3);
  });

  it("reports errors and keeps the previous data", async () => {
    const { result } = renderHook(() =>
      useAsyncResource(() => Promise.reject("boom"), [], {
        initial: "kept",
        errorMessage: "Failed to load",
      }),
    );
    await flush();

    expect(result.current).toMatchObject({
      data: "kept",
      loading: false,
      error: "Failed to load",
    });
  });

  it("resets to initial without fetching when the fetcher is null", async () => {
    const { result } = renderHook(() =>
      useAsyncResource<string>(null, [], { initial: "idle" }),
    );
    await flush();

    expect(result.current).toMatchObject({ data: "idle", loading: false });
  });
});
