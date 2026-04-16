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
  fetchSessions: vi.fn(),
}));

import { fetchActivity } from "./api";
import { fetchSessions, fetchAllTabs } from "../services/api";

describe("ActivityPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    const now = new Date().toISOString();
    useAppStore.setState({
      agents: [
        {
          id: "cli",
          name: "CLI",
          connectedAt: "2026-03-16T08:00:00Z",
          lastActivity: "2026-03-16T08:10:00Z",
          requestCount: 3,
        },
      ],
      agentEventsById: {},
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
          mode: "headless",
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
    vi.mocked(fetchSessions).mockResolvedValue([]);
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
          timestamp: now,
          source: "client",
          requestId: "req_123",
          agentId: "cli",
          instanceId: "inst_123",
          profileName: "default",
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

    expect(screen.getByText("Click on page")).toBeInTheDocument();
    expect(screen.getByText("200")).toBeInTheDocument();
    expect(screen.getAllByText("tab:tab_123").length).toBeGreaterThan(0);
  });

  it("starts unfiltered showing all client events", async () => {
    render(
      <MemoryRouter>
        <ActivityPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalledWith(
        expect.objectContaining({
          source: "client",
          limit: 1000,
        }),
      );
    });

    const query = vi.mocked(fetchActivity).mock.calls[0][0];
    expect(query).not.toHaveProperty("profileName");
    expect(query).not.toHaveProperty("instanceId");
    expect(query).not.toHaveProperty("tabId");
    expect(query).not.toHaveProperty("ageSec");
  });

  it("clears back to unfiltered state", async () => {
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

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenLastCalledWith(
        expect.objectContaining({ instanceId: "inst_123" }),
      );
    });

    await userEvent.click(screen.getByRole("button", { name: "Clear" }));

    await waitFor(() => {
      const lastQuery = vi.mocked(fetchActivity).mock.lastCall?.[0];
      expect(lastQuery).not.toHaveProperty("instanceId");
      expect(lastQuery).not.toHaveProperty("profileName");
    });
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
    expect(screen.queryByLabelText("Session")).not.toBeInTheDocument();
  });
});
