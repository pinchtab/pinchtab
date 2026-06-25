import { describe, it, expect } from "vitest";
import {
  filterMatchingEvents,
  filterAgentThreadEvents,
  partitionCatalogEvents,
  partitionAgentThreadEvents,
  normalizeDashboardActivityEvent,
} from "./selectors";
import { withClearedSessionFilter } from "./helpers";
import type { ActivityFilters, DashboardActivityEvent } from "./types";

function evt(
  partial: Partial<DashboardActivityEvent> & { timestamp: string },
): DashboardActivityEvent {
  return normalizeDashboardActivityEvent({
    channel: "tool_call",
    message: "",
    timestamp: partial.timestamp,
    source: partial.source ?? "client",
    requestId: partial.requestId ?? partial.timestamp,
    sessionId: partial.sessionId ?? "",
    agentId: partial.agentId ?? "",
    method: "GET",
    path: partial.path ?? "/x",
    status: 200,
    instanceId: partial.instanceId ?? "",
    profileName: partial.profileName ?? "",
    tabId: partial.tabId ?? "",
    action: partial.action ?? "",
  } as DashboardActivityEvent);
}

const fixture: DashboardActivityEvent[] = [
  evt({ timestamp: "2024-01-01T00:00:00Z", agentId: "a1", sessionId: "s1" }),
  evt({ timestamp: "2024-01-01T00:01:00Z", agentId: "a1", sessionId: "s2" }),
  evt({ timestamp: "2024-01-01T00:02:00Z", agentId: "a2", sessionId: "s3" }),
  evt({ timestamp: "2024-01-01T00:03:00Z", agentId: "a2", sessionId: "" }),
  // non-client: excluded by matchesVisibleEvent regardless of filters
  evt({ timestamp: "2024-01-01T00:04:00Z", agentId: "a1", source: "server" }),
];

const filterCases: ActivityFilters[] = [
  { agentId: "", sessionId: "" } as ActivityFilters,
  { agentId: "a1", sessionId: "" } as ActivityFilters,
  { agentId: "a1", sessionId: "s1" } as ActivityFilters,
  { agentId: "a2", sessionId: "s3" } as ActivityFilters,
];

describe("partitionCatalogEvents", () => {
  it("matches the three legacy filterMatchingEvents calls", () => {
    for (const filters of filterCases) {
      const got = partitionCatalogEvents(fixture, filters, [], false);

      const visible = filterMatchingEvents(fixture, filters, [], false);
      const sessionCatalog = filterMatchingEvents(
        fixture,
        withClearedSessionFilter(filters),
        [],
        false,
      );
      const agentCatalog = filterMatchingEvents(
        fixture,
        { ...withClearedSessionFilter(filters), agentId: "" },
        [],
        false,
      );

      expect(got.visibleEvents).toEqual(visible);
      expect(got.sessionCatalogEvents).toEqual(sessionCatalog);
      expect(got.agentCatalogEvents).toEqual(agentCatalog);
    }
  });
});

describe("partitionAgentThreadEvents", () => {
  it("matches the two legacy filterAgentThreadEvents calls", () => {
    for (const filters of filterCases) {
      const got = partitionAgentThreadEvents(fixture, filters, [], false);

      const threadEvents = filterAgentThreadEvents(fixture, filters, [], false);
      const threadSessionCatalog = filterAgentThreadEvents(
        fixture,
        withClearedSessionFilter(filters),
        [],
        false,
      );

      expect(got.agentThreadEvents).toEqual(threadEvents);
      expect(got.agentThreadSessionCatalogEvents).toEqual(threadSessionCatalog);
    }
  });
});
