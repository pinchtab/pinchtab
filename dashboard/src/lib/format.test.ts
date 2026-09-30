import { describe, expect, it } from "vitest";
import { formatClockTime, formatUptime } from "./format";

describe("formatClockTime", () => {
  const at = new Date(2026, 0, 2, 9, 5, 7).getTime();

  it("renders HH:MM:SS by default", () => {
    expect(formatClockTime(at)).toBe("09:05:07");
  });

  it("drops seconds on request", () => {
    expect(
      formatClockTime(new Date(at).toISOString(), { seconds: false }),
    ).toBe("09:05");
  });
});

describe("formatUptime", () => {
  const start = "2026-01-01T00:00:00Z";
  const startMs = Date.parse(start);

  it.each([
    [-1000, "just now"],
    [42_000, "42s"],
    [5 * 60_000, "5m"],
    [(2 * 60 + 3) * 60_000, "2h 3m"],
    [(24 * 3 + 4) * 3_600_000, "3d 4h"],
  ])("after %ims reads %s", (elapsed, expected) => {
    expect(formatUptime(start, startMs + elapsed)).toBe(expected);
  });
});
