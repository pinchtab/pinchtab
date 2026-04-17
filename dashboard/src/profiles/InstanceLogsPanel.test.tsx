import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import InstanceLogsPanel from "./InstanceLogsPanel";

const { fetchInstanceLogs, subscribeToInstanceLogs } = vi.hoisted(() => ({
  fetchInstanceLogs: vi.fn(),
  subscribeToInstanceLogs: vi.fn(),
}));

vi.mock("../services/api", () => ({
  fetchInstanceLogs,
  subscribeToInstanceLogs,
}));

beforeEach(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

describe("InstanceLogsPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    subscribeToInstanceLogs.mockReturnValue(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("loads logs on mount when an instance id is provided", async () => {
    fetchInstanceLogs.mockResolvedValue("first line\nsecond line");

    render(<InstanceLogsPanel instanceId="inst_123" />);

    expect(screen.getByText("Loading logs...")).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("first line")).toBeInTheDocument();
      expect(screen.getByText("second line")).toBeInTheDocument();
    });
    expect(fetchInstanceLogs).toHaveBeenCalledWith("inst_123");
    expect(subscribeToInstanceLogs).toHaveBeenCalledWith("inst_123", {
      onLogs: expect.any(Function),
    });
  });

  it("updates rendered logs from the subscription stream", async () => {
    fetchInstanceLogs.mockResolvedValue("");
    let onLogs: ((logs: string) => void) | undefined;

    subscribeToInstanceLogs.mockImplementation((_id, handlers) => {
      onLogs = handlers.onLogs;
      return () => {};
    });

    render(<InstanceLogsPanel instanceId="inst_123" />);

    await waitFor(() => {
      expect(subscribeToInstanceLogs).toHaveBeenCalledTimes(1);
    });

    await act(async () => {
      onLogs?.("streamed logs");
    });

    expect(screen.getByText("streamed logs")).toBeInTheDocument();
  });

  it("keeps streamed logs when the initial fetch resolves late", async () => {
    let onLogs: ((logs: string) => void) | undefined;
    let resolveInitialFetch: ((value: string) => void) | undefined;

    fetchInstanceLogs.mockImplementation(
      () =>
        new Promise<string>((resolve) => {
          resolveInitialFetch = resolve;
        }),
    );
    subscribeToInstanceLogs.mockImplementation((_id, handlers) => {
      onLogs = handlers.onLogs;
      return () => {};
    });

    render(<InstanceLogsPanel instanceId="inst_123" />);

    await waitFor(() => {
      expect(subscribeToInstanceLogs).toHaveBeenCalledTimes(1);
    });

    await act(async () => {
      onLogs?.("fresh stream logs");
    });

    await act(async () => {
      resolveInitialFetch?.("stale initial logs");
    });

    expect(screen.getByText("fresh stream logs")).toBeInTheDocument();
    expect(screen.queryByText("stale initial logs")).not.toBeInTheDocument();
  });

  it("shows the empty state when no instance is available", () => {
    render(<InstanceLogsPanel />);

    expect(screen.getByText("No instance logs available.")).toBeInTheDocument();
    expect(fetchInstanceLogs).not.toHaveBeenCalled();
    expect(subscribeToInstanceLogs).not.toHaveBeenCalled();
  });
});
