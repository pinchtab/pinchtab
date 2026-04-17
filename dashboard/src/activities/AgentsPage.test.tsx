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
    vi.mocked(fetchSessions).mockResolvedValue([
      {
        id: "ses_123",
        agentId: "cli",
        label: "Checkout flow",
        createdAt: "2026-03-16T08:59:00Z",
        lastSeenAt: "2026-03-16T09:05:00Z",
        expiresAt: "",
        status: "active",
      },
    ]);
    vi.mocked(fetchAgent).mockResolvedValue({
      agent: {
        id: "cli",
        name: "CLI",
        connectedAt: "2026-03-16T08:00:00Z",
        lastActivity: "2026-03-16T08:10:00Z",
        requestCount: 3,
      },
      events: [],
    });
    vi.mocked(fetchActivity).mockResolvedValue({
      count: 4,
      events: [
        {
          timestamp: "2026-03-16T09:00:00Z",
          source: "client",
          requestId: "req_123",
          sessionId: "ses_123",
          agentId: "cli",
          method: "POST",
          path: "/tabs/tab_123/action",
          status: 200,
          durationMs: 87,
          tabId: "tab_123",
          action: "click",
        },
        {
          timestamp: "2026-03-16T09:00:01Z",
          source: "client",
          requestId: "req_124",
          agentId: "cli",
          method: "GET",
          path: "/text",
          status: 200,
          durationMs: 22,
          tabId: "tab_123",
          action: "text",
        },
        {
          timestamp: "2026-03-16T09:00:02Z",
          source: "server",
          requestId: "req_125",
          agentId: "cli",
          method: "GET",
          path: "/tabs/tab_123/text",
          status: 200,
          durationMs: 11,
          tabId: "tab_123",
        },
        {
          timestamp: "2026-03-16T09:00:03Z",
          source: "dashboard",
          requestId: "req_hidden",
          agentId: "cli",
          method: "POST",
          path: "/tabs/tab_123/navigate",
          status: 200,
          durationMs: 9,
          tabId: "tab_123",
          url: "https://hidden.example",
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
      expect(fetchActivity).toHaveBeenCalledWith(
        expect.objectContaining({
          source: "client",
          limit: 1000,
        }),
      );
    });
    expect(vi.mocked(fetchActivity).mock.calls[0]?.[0]).not.toHaveProperty(
      "agentId",
    );
    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalledWith(
        expect.objectContaining({
          source: "client",
          limit: 1000,
          agentId: "cli",
        }),
      );
    });

    expect(screen.getByRole("button", { name: "Agents" })).toHaveClass(
      "bg-primary/8",
    );
    expect(screen.queryByText("Request timeline")).not.toBeInTheDocument();
  });

  it("switches to Activities and shows the filter stack including agent filter", async () => {
    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalled();
    });

    await userEvent.click(screen.getByRole("button", { name: "Activities" }));

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalled();
    });

    await waitFor(() => {
      const lastQuery = vi.mocked(fetchActivity).mock.lastCall?.[0];
      expect(lastQuery).not.toHaveProperty("agentId");
    });

    expect(screen.getByLabelText("Profile")).toBeInTheDocument();
    expect(screen.getByLabelText("Agent")).toBeInTheDocument();
  });

  it("keeps the simplified event rows and copyable tab ids", async () => {
    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("Click on page")).toBeInTheDocument();
    });

    expect(
      screen.queryByRole("button", { name: "All Agents" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("bridge")).not.toBeInTheDocument();
    expect(screen.queryByText("POST")).not.toBeInTheDocument();
    expect(screen.getAllByText("200").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Checkout flow").length).toBeGreaterThan(0);
    expect(screen.getAllByTitle(/Copy tab ID tab_123/).length).toBeGreaterThan(
      0,
    );
  });

  it("shows only client-sourced agent activity in the thread", async () => {
    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("Click on page")).toBeInTheDocument();
    });
    expect(screen.getByText("Extract text from page")).toBeInTheDocument();
    expect(screen.getAllByText("anonymous").length).toBeGreaterThan(0);
    expect(
      screen.queryByText("Navigate to https://hidden.example"),
    ).not.toBeInTheDocument();
  });

  it("updates the open agent thread and sidebar list from the shared store", async () => {
    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalled();
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
    expect(screen.queryByText("Planning next step")).not.toBeInTheDocument();
  });

  it("keeps sibling sessions visible after selecting a derived session", async () => {
    vi.mocked(fetchSessions).mockResolvedValue([]);
    vi.mocked(fetchActivity).mockResolvedValue({
      count: 2,
      events: [
        {
          timestamp: "2026-03-16T09:00:00Z",
          source: "client",
          requestId: "req_session_1",
          sessionId: "ses_123",
          agentId: "cli",
          method: "POST",
          path: "/tabs/tab_123/action",
          status: 200,
          durationMs: 87,
          tabId: "tab_123",
          action: "click",
        },
        {
          timestamp: "2026-03-16T09:10:00Z",
          source: "client",
          requestId: "req_session_2",
          sessionId: "ses_456",
          agentId: "cli",
          method: "GET",
          path: "/text",
          status: 200,
          durationMs: 22,
          tabId: "tab_123",
          action: "text",
        },
      ],
    });

    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getAllByRole("button", { name: /^Session / })).toHaveLength(
        2,
      );
    });

    await userEvent.click(
      screen.getAllByRole("button", { name: /^Session / })[0],
    );

    await waitFor(() => {
      expect(screen.getAllByRole("button", { name: /^Session / })).toHaveLength(
        2,
      );
    });
  });

  it("shows client events without agent ids as anonymous", async () => {
    useAppStore.setState({
      agents: [],
      agentEventsById: {},
      profiles: [],
      instances: [],
      currentTabs: {},
    });
    vi.mocked(fetchActivity).mockImplementation(async (query) => {
      if (query?.agentId && query.agentId !== "anonymous") {
        return { count: 0, events: [] };
      }
      return {
        count: 1,
        events: [
          {
            timestamp: "2026-04-08T18:02:26.983381Z",
            source: "client",
            requestId: "66f833fb1a65b720",
            method: "POST",
            path: "/navigate",
            status: 200,
            durationMs: 1066,
            instanceId: "inst_990e5062",
            profileId: "prof_37a8eec1",
            profileName: "default",
            tabId: "ACDF218287DBE662F237F064618A624D",
            url: "https://github.com/",
            action: "navigate",
          },
        ],
      };
    });

    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(
        screen.getByText("Navigate to https://github.com/"),
      ).toBeInTheDocument();
    });

    expect(screen.getAllByText("anonymous").length).toBeGreaterThan(0);
  });

  it("hydrates the selected agent thread from a scoped activity query", async () => {
    vi.mocked(fetchActivity).mockImplementation(async (query) => {
      if (query?.agentId === "cli") {
        return {
          count: 1,
          events: [
            {
              timestamp: "2026-03-16T07:45:00Z",
              source: "client",
              requestId: "req_thread_only",
              sessionId: "ses_789",
              agentId: "cli",
              method: "GET",
              path: "/text",
              status: 200,
              durationMs: 14,
              tabId: "tab_123",
              action: "text",
            },
          ],
        };
      }
      return {
        count: 0,
        events: [],
      };
    });

    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchAgent).toHaveBeenCalledWith("cli");
    });

    await waitFor(() => {
      expect(fetchActivity).toHaveBeenCalledWith(
        expect.objectContaining({
          source: "client",
          limit: 1000,
          agentId: "cli",
        }),
      );
    });

    expect(screen.getByText("Extract text from page")).toBeInTheDocument();
  });

  it("hydrates older agent history from the agent detail endpoint", async () => {
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
          id: "evt_thread_history",
          timestamp: "2026-03-16T07:45:00Z",
          agentId: "cli",
          channel: "tool_call",
          type: "text",
          method: "GET",
          path: "/text",
          message: "",
          details: {
            source: "client",
            requestId: "req_thread_history",
            sessionId: "ses_789",
            status: 200,
            durationMs: 14,
            tabId: "tab_123",
            action: "text",
          },
        },
      ],
    });
    vi.mocked(fetchActivity).mockResolvedValue({
      count: 0,
      events: [],
    });

    render(
      <MemoryRouter>
        <AgentsPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(fetchAgent).toHaveBeenCalledWith("cli");
    });

    expect(screen.getByText("Extract text from page")).toBeInTheDocument();
  });
});
