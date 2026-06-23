import { describe, it, expect } from "vitest";
import {
  mergeAgents,
  handoffFromSystemEvent,
  resumeTabIdFromSystemEvent,
} from "./realtimeReducers";
import type { SystemEvent } from "./api";

describe("mergeAgents", () => {
  it("merges agents with the same id keeping earliest connect / latest activity / max requests", () => {
    const current = [
      {
        id: "a",
        name: "a",
        connectedAt: "2024-01-01T00:05:00Z",
        lastActivity: "2024-01-01T00:10:00Z",
        requestCount: 3,
      },
    ] as any;
    const incoming = [
      {
        id: "a",
        name: "a",
        connectedAt: "2024-01-01T00:01:00Z", // earlier connect wins
        lastActivity: "2024-01-01T00:08:00Z", // older than current -> current wins
        requestCount: 7, // max wins
      },
    ] as any;

    const [merged] = mergeAgents(current, incoming);
    expect(merged.connectedAt).toBe("2024-01-01T00:01:00Z");
    expect(merged.lastActivity).toBe("2024-01-01T00:10:00Z");
    expect(merged.requestCount).toBe(7);
  });

  it("keeps distinct ids and orders most-recently-active first", () => {
    const current = [
      {
        id: "old",
        name: "old",
        connectedAt: "2024-01-01T00:00:00Z",
        lastActivity: "2024-01-01T00:00:00Z",
        requestCount: 1,
      },
    ] as any;
    const incoming = [
      {
        id: "new",
        name: "new",
        connectedAt: "2024-01-01T01:00:00Z",
        lastActivity: "2024-01-01T01:00:00Z",
        requestCount: 1,
      },
    ] as any;

    const merged = mergeAgents(current, incoming);
    expect(merged.map((a) => a.id)).toEqual(["new", "old"]);
  });
});

describe("handoffFromSystemEvent", () => {
  it("maps a valid tab.handoff event with defaults", () => {
    const event: SystemEvent = {
      type: "tab.handoff",
      instance: {
        tabId: "tab1",
        hint: "solve captcha",
        source: "agent",
      } as any,
    };
    const notification = handoffFromSystemEvent(event);
    expect(notification).not.toBeNull();
    expect(notification?.tabId).toBe("tab1");
    expect(notification?.reason).toBe("manual_handoff"); // default
    expect(notification?.hint).toBe("solve captcha");
    expect(notification?.source).toBe("agent");
    expect(typeof notification?.receivedAt).toBe("number");
  });

  it("returns null when the tab id is missing or not a string", () => {
    expect(
      handoffFromSystemEvent({ type: "tab.handoff", instance: {} as any }),
    ).toBeNull();
    expect(
      handoffFromSystemEvent({
        type: "tab.handoff",
        instance: { tabId: 5 } as any,
      }),
    ).toBeNull();
  });

  it("returns null for non-handoff events", () => {
    expect(
      handoffFromSystemEvent({
        type: "tab.resume",
        instance: { tabId: "t" } as any,
      }),
    ).toBeNull();
    expect(handoffFromSystemEvent({ type: "instance.started" })).toBeNull();
  });
});

describe("resumeTabIdFromSystemEvent", () => {
  it("returns the tab id for a tab.resume event", () => {
    expect(
      resumeTabIdFromSystemEvent({
        type: "tab.resume",
        instance: { tabId: "tab9" } as any,
      }),
    ).toBe("tab9");
  });

  it("returns null when missing or for non-resume events", () => {
    expect(
      resumeTabIdFromSystemEvent({ type: "tab.resume", instance: {} as any }),
    ).toBeNull();
    expect(
      resumeTabIdFromSystemEvent({
        type: "tab.handoff",
        instance: { tabId: "t" } as any,
      }),
    ).toBeNull();
  });
});
