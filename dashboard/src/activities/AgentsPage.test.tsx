import { act, render, screen, waitFor } from "@testing-library/react";
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
  fetchAgent: vi.fn(),
  fetchSessions: vi.fn(),
}));

import { fetchActivity } from "./api";
import { fetchAgent, fetchSessions, fetchAllTabs } from "../services/api";

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
      agentEventsById: {},
      profiles: [],
      instances: [],
      currentTabs: {},
    });
    vi.mocked(fetchAllTabs).mockResolvedValue([]);
    vi.mocked(fetchSessions).mockResolvedValue([]);
    vi.mocked(fetchAgent).mockResolvedValue({
      agent: {
        id: "cli",
        name: "CLI",
        connectedAt: "2026-03-16T08:00:00Z",
        lastActivity: "2026-03-16T08:10:00Z",
        requestCount: 3,
      },
      events: [
        {
          id: "evt_1",
          agentId: "cli",
          channel: "tool_call",
          type: "click",
          method: "POST",
          path: "/tabs/tab_123/action",
          timestamp: "2026-03-16T09:00:00Z",
          details: {
            source: "bridge",
            requestId: "req_123",
            status: 200,
            durationMs: 87,
            tabId: "tab_123",
            action: "click",
          },
        },
        {
          id: "evt_2",
          agentId: "cli",
          channel: "tool_call",
          type: "text",
          method: "GET",
          path: "/tabs/tab_123/text",
          timestamp: "2026-03-16T09:00:02Z",
          details: {
            source: "server",
            requestId: "req_125",
            status: 200,
            durationMs: 11,
            tabId: "tab_123",
          },
        },
        {
          id: "evt_hidden",
          agentId: "cli",
          channel: "tool_call",
          type: "navigate",
          method: "POST",
          path: "/tabs/tab_123/navigate",
          timestamp: "2026-03-16T09:00:03Z",
          details: {
            source: "dashboard",
            requestId: "req_hidden",
            status: 200,
            durationMs: 9,
            tabId: "tab_123",
            url: "https://hidden.example",
            action: "navigate",
          },
        },
      ],
    });
    vi.mocked(fetchActivity).mockResolvedValue({
      count: 1,
      events: [
        {
          timestamp: "2026-03-16T09:01:00Z",
          source: "bridge",
          requestId: "req_activity",
          agentId: "cli",
          method: "POST",
          path: "/tabs/tab_555/navigate",
          status: 200,
          durationMs: 10,
          tabId: "tab_555",
          url: "https://pinchtab.com",
          action: "navigate",
        },
      ],
    });
  });

  it("defaults the right rail to Agents and bootstraps the selected agent thread", async () => {
    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchAgent).toHaveBeenCalledWith("cli", "both");
    });

    expect(screen.getByRole("button", { name: "Agents" })).toHaveClass(
      "bg-primary/8",
    );
    expect(screen.queryByText("Request timeline")).not.toBeInTheDocument();
    expect(fetchActivity).not.toHaveBeenCalled();
  });

  it("switches to Activities and shows the filter stack including agent filter", async () => {
    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchAgent).toHaveBeenCalled();
    });

    await userEvent.click(screen.getByRole("button", { name: "Activities" }));

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalled();
    });

    expect(screen.getByLabelText("Profile")).toBeInTheDocument();
    expect(screen.getByLabelText("Agent")).toBeInTheDocument();
  });

  it("keeps the simplified event rows and inline copyable tab ids", async () => {
    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText(/Click on page on tab/)).toBeInTheDocument();
    });

    expect(
      screen.queryByRole("button", { name: "All Agents" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("bridge")).not.toBeInTheDocument();
    expect(screen.queryByText("POST")).not.toBeInTheDocument();
    expect(screen.queryByText("200")).not.toBeInTheDocument();
    expect(screen.queryByText("agent:cli")).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "tab_123" }).length).toBe(3);
  });

  it("shows all agent activity in the thread regardless of source", async () => {
    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText(/3 events • 1 agents/)).toBeInTheDocument();
    });

    expect(
      screen.getAllByText(
        (_content, element) =>
          element?.textContent === "Click on page on tab tab_123",
      ).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText(
        (_content, element) =>
          element?.textContent === "Extract text from page on tab tab_123",
      ).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText(/Navigate to https:\/\/hidden\.example/).length,
    ).toBeGreaterThan(0);
  });

  it("updates the open agent thread and sidebar list from the shared store", async () => {
    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchAgent).toHaveBeenCalled();
    });

    act(() => {
      useAppStore.getState().upsertAgentFromEvent({
        id: "evt_new_agent",
        agentId: "worker-2",
        channel: "progress",
        type: "progress",
        method: "POST",
        path: "/navigate",
        message: "Planning next step",
        timestamp: "2026-03-16T09:00:04Z",
      } as any);
      useAppStore.getState().appendAgentEvent({
        id: "evt_live",
        agentId: "cli",
        channel: "progress",
        type: "progress",
        method: "POST",
        path: "/action",
        message: "Planning next step",
        timestamp: "2026-03-16T09:00:05Z",
        details: {
          source: "server",
          requestId: "req_live",
          status: 201,
          durationMs: 3,
        },
      } as any);
    });

    expect(screen.getByText("worker-2")).toBeInTheDocument();
    expect(screen.getByText("Planning next step")).toBeInTheDocument();
  });
});
