import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import InstanceLogsPanel from "./InstanceLogsPanel";

const { fetchInstanceLogs, subscribeToInstanceLogs } = vi.hoisted(() => ({
  fetchInstanceLogs: vi.fn(),
  subscribeToInstanceLogs: vi.fn(),
}));

vi.mock("../../services/api", () => ({
  fetchInstanceLogs,
  subscribeToInstanceLogs,
}));

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
      expect(document.querySelector("pre")).toHaveTextContent(
        "first line second line",
      );
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

    expect(document.querySelector("pre")).toHaveTextContent("streamed logs");
  });

  it("shows the empty state when no instance is available", () => {
    render(<InstanceLogsPanel />);

    expect(screen.getByText("No instance logs available.")).toBeInTheDocument();
    expect(fetchInstanceLogs).not.toHaveBeenCalled();
    expect(subscribeToInstanceLogs).not.toHaveBeenCalled();
  });
});
