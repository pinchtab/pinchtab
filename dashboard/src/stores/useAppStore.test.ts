import { describe, it, expect, beforeEach } from "vitest";
import { useAppStore } from "./useAppStore";

describe("useAppStore", () => {
  beforeEach(() => {
    // Reset store between tests
    useAppStore.setState({
      profiles: [],
      profilesLoading: false,
      instances: [],
      instancesLoading: false,
      tabsChartData: [],
      memoryChartData: [],
      serverChartData: [],
      currentTabs: {},
      currentMemory: {},
      agents: [],
      selectedAgentId: null,
      agentEventsById: {},
      events: [],
      eventFilter: "all",
      settings: {
        screencast: { fps: 1, quality: 30, maxWidth: 800 },
        stealth: "light",
        browser: {
          blockImages: false,
          blockMedia: false,
          noAnimations: false,
        },
        monitoring: { memoryMetrics: false, pollInterval: 30 },
        agents: { reasoningMode: "tool_calls" },
      },
      serverInfo: null,
    });
  });

  describe("profiles", () => {
    it("sets profiles", () => {
      const profiles = [{ name: "test", id: "prof_123" }] as any;
      useAppStore.getState().setProfiles(profiles);
      expect(useAppStore.getState().profiles).toEqual(profiles);
    });

    it("sets profiles loading state", () => {
      useAppStore.getState().setProfilesLoading(true);
      expect(useAppStore.getState().profilesLoading).toBe(true);
    });
  });

  describe("instances", () => {
    it("sets instances", () => {
      const instances = [{ id: "inst_123", profileName: "test" }] as any;
      useAppStore.getState().setInstances(instances);
      expect(useAppStore.getState().instances).toEqual(instances);
    });
  });

  describe("chart data", () => {
    it("adds chart data point", () => {
      const point = { timestamp: Date.now(), inst_123: 5 };
      useAppStore.getState().addChartDataPoint(point);
      expect(useAppStore.getState().tabsChartData).toHaveLength(1);
      expect(useAppStore.getState().tabsChartData[0]).toEqual(point);
    });

    it("keeps only last 60 data points", () => {
      // Add 65 points
      for (let i = 0; i < 65; i++) {
        useAppStore.getState().addChartDataPoint({ timestamp: i, inst_123: i });
      }

      const data = useAppStore.getState().tabsChartData;
      expect(data).toHaveLength(60);
      // Should have points 5-64 (the last 60)
      expect(data[0].timestamp).toBe(5);
      expect(data[59].timestamp).toBe(64);
    });

    it("sets current tabs", () => {
      const tabs = {
        inst_123: [{ id: "tab_1", url: "https://pinchtab.com" }],
      } as any;
      useAppStore.getState().setCurrentTabs(tabs);
      expect(useAppStore.getState().currentTabs).toEqual(tabs);
    });
  });

  describe("events", () => {
    it("adds event to beginning of list", () => {
      const event1 = { type: "action", timestamp: "2024-01-01" } as any;
      const event2 = { type: "action", timestamp: "2024-01-02" } as any;

      useAppStore.getState().addEvent(event1);
      useAppStore.getState().addEvent(event2);

      const events = useAppStore.getState().events;
      expect(events[0]).toEqual(event2); // Most recent first
      expect(events[1]).toEqual(event1);
    });

    it("limits events to 100", () => {
      // Add 105 events
      for (let i = 0; i < 105; i++) {
        useAppStore.getState().addEvent({ type: "action", id: i } as any);
      }

      const events = useAppStore.getState().events;
      expect(events).toHaveLength(100);
      // Most recent should be id: 104
      expect(events[0].id).toBe(104);
    });

    it("clears events", () => {
      useAppStore.getState().addEvent({ type: "action" } as any);
      useAppStore.getState().clearEvents();
      expect(useAppStore.getState().events).toHaveLength(0);
    });

    it("sets event filter", () => {
      useAppStore.getState().setEventFilter("errors");
      expect(useAppStore.getState().eventFilter).toBe("errors");
    });
  });

  describe("agents", () => {
    it("sets agents", () => {
      const agents = [{ id: "agent_1", name: "Test Agent" }] as any;
      useAppStore.getState().setAgents(agents);
      expect(useAppStore.getState().agents).toEqual(agents);
    });

    it("sets selected agent id", () => {
      useAppStore.getState().setSelectedAgentId("agent_1");
      expect(useAppStore.getState().selectedAgentId).toBe("agent_1");
    });

    it("clears selected agent id", () => {
      useAppStore.getState().setSelectedAgentId("agent_1");
      useAppStore.getState().setSelectedAgentId(null);
      expect(useAppStore.getState().selectedAgentId).toBeNull();
    });

    it("upserts agent details from live events", () => {
      useAppStore.getState().upsertAgentFromEvent({
        id: "evt_1",
        agentId: "agent_1",
        channel: "progress",
        type: "progress",
        method: "",
        path: "",
        message: "Thinking",
        timestamp: "2024-01-01T00:00:00Z",
      } as any);

      let agents = useAppStore.getState().agents;
      expect(agents).toHaveLength(1);
      expect(agents[0].id).toBe("agent_1");
      expect(agents[0].requestCount).toBe(1);

      useAppStore.getState().upsertAgentFromEvent({
        id: "evt_2",
        agentId: "agent_1",
        channel: "tool_call",
        type: "navigate",
        method: "POST",
        path: "/navigate",
        timestamp: "2024-01-01T00:01:00Z",
      } as any);

      agents = useAppStore.getState().agents;
      expect(agents[0].requestCount).toBe(2);
      expect(agents[0].lastActivity).toBe("2024-01-01T00:01:00Z");
    });

    it("ignores live events without an agent id", () => {
      useAppStore.getState().upsertAgentFromEvent({
        id: "evt_1",
        agentId: "",
        channel: "progress",
        type: "progress",
        method: "",
        path: "",
        timestamp: "2024-01-01T00:00:00Z",
      } as any);

      expect(useAppStore.getState().agents).toEqual([]);
    });

    it("hydrates agent history without dropping already streamed events", () => {
      useAppStore.getState().appendAgentEvent({
        id: "evt_live",
        agentId: "agent_1",
        channel: "progress",
        type: "progress",
        method: "POST",
        path: "/api/agents/agent_1/events",
        timestamp: "2024-01-01T00:02:00Z",
      } as any);

      useAppStore.getState().hydrateAgentEvents("agent_1", [
        {
          id: "evt_old",
          agentId: "agent_1",
          channel: "tool_call",
          type: "navigate",
          method: "POST",
          path: "/navigate",
          timestamp: "2024-01-01T00:01:00Z",
        } as any,
        {
          id: "evt_live",
          agentId: "agent_1",
          channel: "progress",
          type: "progress",
          method: "POST",
          path: "/api/agents/agent_1/events",
          timestamp: "2024-01-01T00:02:00Z",
        } as any,
      ]);

      expect(useAppStore.getState().agentEventsById.agent_1).toEqual([
        expect.objectContaining({ id: "evt_old" }),
        expect.objectContaining({ id: "evt_live" }),
      ]);
    });

    it("keeps only the 20 most recent agent caches", () => {
      for (let i = 0; i < 22; i++) {
        const agentId = `agent_${i}`;
        const timestamp = new Date(Date.UTC(2024, 0, 1, 0, i)).toISOString();
        useAppStore.getState().upsertAgentFromEvent({
          id: `evt_${i}`,
          agentId,
          channel: "tool_call",
          type: "navigate",
          method: "POST",
          path: "/navigate",
          timestamp,
        } as any);
        useAppStore.getState().appendAgentEvent({
          id: `evt_cache_${i}`,
          agentId,
          channel: "tool_call",
          type: "navigate",
          method: "POST",
          path: "/navigate",
          timestamp,
        } as any);
      }

      const { agents, agentEventsById } = useAppStore.getState();
      expect(agents).toHaveLength(20);
      expect(Object.keys(agentEventsById)).toHaveLength(20);
      expect(agents.map((agent) => agent.id)).not.toContain("agent_0");
      expect(agents.map((agent) => agent.id)).not.toContain("agent_1");
      expect(agentEventsById.agent_0).toBeUndefined();
      expect(agentEventsById.agent_1).toBeUndefined();
      expect(agentEventsById.agent_21).toBeDefined();
    });

    it("ignores a duplicate appended event without rewriting history", () => {
      const event = {
        id: "evt_dup",
        agentId: "agent_dup",
        channel: "progress",
        type: "progress",
        method: "POST",
        path: "/api/agents/agent_dup/events",
        timestamp: "2024-01-01T00:00:00Z",
      } as any;
      useAppStore.getState().appendAgentEvent(event);
      const before = useAppStore.getState().agentEventsById;
      useAppStore.getState().appendAgentEvent(event);
      const after = useAppStore.getState().agentEventsById;
      // Same reference proves the early return state (no re-merge / re-sort).
      expect(after).toBe(before);
      expect(after.agent_dup).toHaveLength(1);
    });

    it("hydrates events in timestamp order regardless of input order", () => {
      const make = (id: string, timestamp: string) =>
        ({
          id,
          agentId: "agent_order",
          channel: "progress",
          type: "progress",
          method: "POST",
          path: "/p",
          timestamp,
        }) as any;
      useAppStore.getState().hydrateAgentEvents("agent_order", [
        make("evt_b", "2024-01-01T00:03:00Z"),
        make("evt_a", "2024-01-01T00:01:00Z"),
        make("evt_b", "2024-01-01T00:03:00Z"), // duplicate id, dropped
      ]);
      expect(
        useAppStore
          .getState()
          .agentEventsById.agent_order.map((event) => event.id),
      ).toEqual(["evt_a", "evt_b"]);
    });
  });

  describe("settings", () => {
    it("has default settings", () => {
      const settings = useAppStore.getState().settings;
      expect(settings.stealth).toBe("light");
      expect(settings.screencast?.fps).toBe(1);
      expect(settings.monitoring?.memoryMetrics).toBe(false);
      expect(settings.agents?.reasoningMode).toBe("tool_calls");
    });

    it("updates settings", () => {
      const newSettings = {
        screencast: { fps: 5, quality: 50, maxWidth: 1024 },
        stealth: "strict" as const,
        browser: { blockImages: true, blockMedia: true, noAnimations: true },
        monitoring: { memoryMetrics: true, pollInterval: 30 },
        agents: { reasoningMode: "both" as const },
      };
      useAppStore.getState().setSettings(newSettings);
      expect(useAppStore.getState().settings).toEqual(newSettings);
    });

    it("persists settings to localStorage", () => {
      const newSettings = {
        screencast: { fps: 10, quality: 80, maxWidth: 1280 },
        stealth: "full" as const,
        browser: { blockImages: false, blockMedia: false, noAnimations: false },
        monitoring: { memoryMetrics: true, pollInterval: 30 },
        agents: { reasoningMode: "progress" as const },
      };
      useAppStore.getState().setSettings(newSettings);

      const saved = localStorage.getItem("pinchtab_settings");
      expect(saved).toBeTruthy();
      expect(JSON.parse(saved!)).toEqual(newSettings);
    });
  });

  describe("memory chart data", () => {
    it("adds memory data points", () => {
      const point = { timestamp: Date.now(), inst_1: 50.5 };
      useAppStore.getState().addMemoryDataPoint(point);
      expect(useAppStore.getState().memoryChartData).toContainEqual(point);
    });

    it("limits memory data to 60 points", () => {
      for (let i = 0; i < 65; i++) {
        useAppStore.getState().addMemoryDataPoint({ timestamp: i, inst_1: i });
      }
      expect(useAppStore.getState().memoryChartData).toHaveLength(60);
      // Should have dropped first 5, keeping 5-64
      expect(useAppStore.getState().memoryChartData[0].timestamp).toBe(5);
    });

    it("sets current memory", () => {
      const memory = { inst_1: 85.5, inst_2: 120.3 };
      useAppStore.getState().setCurrentMemory(memory);
      expect(useAppStore.getState().currentMemory).toEqual(memory);
    });
  });

  describe("applyMonitoringSnapshot", () => {
    const snapshot = {
      timestamp: 1000,
      instances: [
        { id: "A", status: "running" },
        { id: "B", status: "running" },
        { id: "C", status: "stopped" },
      ],
      tabs: [
        { id: "t1", instanceId: "A" },
        { id: "t2", instanceId: "A" },
        { id: "t3", instanceId: "B" },
        { id: "t4", instanceId: "C" },
      ],
      metrics: [
        { instanceId: "A", jsHeapUsedMB: 10 },
        { instanceId: "A", jsHeapUsedMB: 99 }, // duplicate: first wins
        { instanceId: "B", jsHeapUsedMB: 20 },
      ],
      serverMetrics: {
        goHeapAllocMB: 42,
        goNumGoroutine: 7,
        rateBucketHosts: 3,
      },
    } as any;

    it("indexes tabs/metrics per running instance and includes memory", () => {
      useAppStore.getState().applyMonitoringSnapshot(snapshot, true);
      const s = useAppStore.getState();

      // Only running instances (C excluded); tab order preserved.
      expect(s.currentTabs).toEqual({
        A: [
          { id: "t1", instanceId: "A" },
          { id: "t2", instanceId: "A" },
        ],
        B: [{ id: "t3", instanceId: "B" }],
      });
      // First metrics entry wins for the duplicate (10, not 99).
      expect(s.currentMemory).toEqual({ A: 10, B: 20 });
      expect(s.currentMetrics).toEqual({
        A: { instanceId: "A", jsHeapUsedMB: 10 },
        B: { instanceId: "B", jsHeapUsedMB: 20 },
      });

      const tabData = s.tabsChartData;
      expect(tabData[tabData.length - 1]).toEqual({
        timestamp: 1000,
        A: 2,
        B: 1,
      });
      const memData = s.memoryChartData;
      expect(memData[memData.length - 1]).toEqual({
        timestamp: 1000,
        A: 10,
        B: 20,
      });
      const serverData = s.serverChartData;
      expect(serverData[serverData.length - 1]).toEqual({
        timestamp: 1000,
        goHeapMB: 42,
        goroutines: 7,
        rateBucketHosts: 3,
      });
      expect(s.instances).toEqual(snapshot.instances);
    });

    it("skips metrics entirely when memory is not included", () => {
      useAppStore.getState().applyMonitoringSnapshot(snapshot, false);
      const s = useAppStore.getState();

      expect(s.currentTabs).toEqual({
        A: [
          { id: "t1", instanceId: "A" },
          { id: "t2", instanceId: "A" },
        ],
        B: [{ id: "t3", instanceId: "B" }],
      });
      expect(s.currentMemory).toEqual({});
      expect(s.currentMetrics).toEqual({});
      expect(s.memoryChartData).toHaveLength(0);
      const tabData = s.tabsChartData;
      expect(tabData[tabData.length - 1]).toEqual({
        timestamp: 1000,
        A: 2,
        B: 1,
      });
    });

    it("does not append a tabs chart point when no instance is running", () => {
      const idle = {
        timestamp: 2000,
        instances: [{ id: "C", status: "stopped" }],
        tabs: [],
        metrics: [],
        serverMetrics: {
          goHeapAllocMB: 1,
          goNumGoroutine: 1,
          rateBucketHosts: 1,
        },
      } as any;
      useAppStore.getState().applyMonitoringSnapshot(idle, true);
      const s = useAppStore.getState();

      expect(s.tabsChartData).toHaveLength(0);
      expect(s.memoryChartData).toHaveLength(0);
      // The server chart point is always appended.
      expect(s.serverChartData).toHaveLength(1);
    });
  });
});
