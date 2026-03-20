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
});
