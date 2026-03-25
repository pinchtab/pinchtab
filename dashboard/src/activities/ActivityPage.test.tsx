import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import ActivityPage from "./ActivityPage";
import { useAppStore } from "../stores/useAppStore";

vi.mock("./api", () => ({
  fetchActivity: vi.fn(),
}));

vi.mock("../services/api", () => ({
  fetchAllTabs: vi.fn(),
}));

import { fetchActivity } from "./api";
import { fetchAllTabs } from "../services/api";

describe("ActivityPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAppStore.setState({
      profiles: [
        {
          id: "prof_default",
          name: "default",
          created: "2026-03-16T08:00:00Z",
          lastUsed: "2026-03-16T08:10:00Z",
          diskUsage: 1024,
          running: false,
        },
      ],
      instances: [
        {
          id: "inst_123",
          profileId: "prof_default",
          profileName: "default",
          port: "9988",
          headless: true,
          status: "running",
          startTime: "2026-03-16T08:00:00Z",
          attached: false,
        },
      ],
      currentTabs: {
        inst_123: [
          {
            id: "tab_123",
            instanceId: "inst_123",
            url: "https://example.com",
            title: "Example",
          },
        ],
      },
    });
    vi.mocked(fetchAllTabs).mockResolvedValue([
      {
        id: "tab_123",
        instanceId: "inst_123",
        url: "https://example.com",
        title: "Example",
      },
    ]);
    vi.mocked(fetchActivity).mockResolvedValue({
      count: 1,
      events: [
        {
          timestamp: "2026-03-16T09:00:00Z",
          source: "cli",
          requestId: "req_123",
          agentId: "cli",
          method: "POST",
          path: "/tabs/tab_123/action",
          status: 200,
          durationMs: 87,
          tabId: "tab_123",
          action: "click",
        },
      ],
    });
  });

  it("loads and renders activity records", async () => {
    render(
      <MemoryRouter>
        <ActivityPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalled();
    });

    expect(screen.getByText("/tabs/tab_123/action")).toBeInTheDocument();
    expect(screen.getByText("agent:cli")).toBeInTheDocument();
    expect(screen.getAllByText("tab:tab_123").length).toBeGreaterThan(0);
  });

  it("defaults to the current running profile and tab", async () => {
    render(
      <MemoryRouter>
        <ActivityPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalledWith(
        expect.objectContaining({
          profileName: "default",
          instanceId: "inst_123",
          tabId: "tab_123",
        }),
      );
    });
  });

  it("clears back to default filters instead of reapplying landing context", async () => {
    render(
      <MemoryRouter>
        <ActivityPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalledWith(
        expect.objectContaining({
          profileName: "default",
          instanceId: "inst_123",
          tabId: "tab_123",
        }),
      );
    });

    await userEvent.click(screen.getByRole("button", { name: "Clear" }));

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenLastCalledWith(
        expect.objectContaining({
          ageSec: 3600,
          limit: 200,
        }),
      );
    });

    expect(vi.mocked(fetchActivity).mock.lastCall?.[0]).not.toEqual(
      expect.objectContaining({
        profileName: "default",
        instanceId: "inst_123",
        tabId: "tab_123",
      }),
    );
  });

  it("applies the tab filter from the dropdown filter panel", async () => {
    render(
      <MemoryRouter>
        <ActivityPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalledTimes(1);
    });

    await userEvent.click(
      screen.getByRole("button", { name: /advanced filters/i }),
    );
    await userEvent.selectOptions(
      screen.getByLabelText("Instance"),
      "inst_123",
    );
    await userEvent.selectOptions(screen.getByLabelText("Tab"), "tab_123");

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenLastCalledWith(
        expect.objectContaining({ instanceId: "inst_123", tabId: "tab_123" }),
      );
    });
  });

  it("keeps the primary filters visible and reveals the rest under advanced", async () => {
    render(
      <MemoryRouter>
        <ActivityPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalled();
    });

    expect(screen.getByLabelText("Profile")).toBeInTheDocument();
    expect(screen.getByLabelText("Tab")).toBeInTheDocument();
    expect(screen.getByLabelText("Agent")).toBeInTheDocument();
    expect(screen.getByLabelText("Action")).toBeInTheDocument();
    expect(screen.queryByLabelText("Instance")).not.toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: /advanced filters/i }),
    );

    expect(screen.getByLabelText("Instance")).toBeInTheDocument();
    expect(screen.getByLabelText("Session")).toBeInTheDocument();
    expect(screen.getByLabelText("Source")).toBeInTheDocument();
  });
});
