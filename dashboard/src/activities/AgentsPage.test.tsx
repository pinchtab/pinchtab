import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import AgentsPage from "./AgentsPage";
import { useAppStore } from "../stores/useAppStore";

vi.mock("./api", () => ({
  fetchActivity: vi.fn(),
}));

vi.mock("../services/api", () => ({
  fetchAllTabs: vi.fn(),
}));

import { fetchActivity } from "./api";
import { fetchAllTabs } from "../services/api";

describe("AgentsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
      profiles: [],
      instances: [],
      currentTabs: {},
    });
    vi.mocked(fetchAllTabs).mockResolvedValue([]);
    vi.mocked(fetchActivity).mockResolvedValue({
      count: 3,
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
          action: "click",
        },
        {
          timestamp: "2026-03-16T09:00:01Z",
          source: "dashboard",
          requestId: "req_124",
          method: "GET",
          path: "/profiles",
          status: 200,
          durationMs: 22,
        },
        {
          timestamp: "2026-03-16T09:00:02Z",
          source: "server",
          requestId: "req_125",
          method: "GET",
          path: "/health",
          status: 200,
          durationMs: 11,
        },
      ],
    });
  });

  it("defaults the right rail to Agents", async () => {
    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalled();
    });

    expect(screen.getByRole("button", { name: "Agents" })).toHaveClass(
      "bg-primary/8",
    );
    expect(screen.queryByText("Request timeline")).not.toBeInTheDocument();
  });

  it("switches to Activities and shows the filter stack", async () => {
    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalled();
    });

    await userEvent.click(screen.getByRole("button", { name: "Activities" }));

    expect(screen.getByLabelText("Profile")).toBeInTheDocument();
    expect(screen.getByLabelText("Agent")).toBeInTheDocument();
  });

  it("filters dashboard and server source events out of the agents stream", async () => {
    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalled();
    });

    expect(screen.getByText("/tabs/tab_123/action")).toBeInTheDocument();
    expect(screen.queryByText("/profiles")).not.toBeInTheDocument();
    expect(screen.queryByText("/health")).not.toBeInTheDocument();
    expect(screen.getByText(/1 events • 1 agents/)).toBeInTheDocument();
  });
});
