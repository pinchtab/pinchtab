import { describe, it, expect } from "vitest";
import { deepEqual } from "./deepEqual";

describe("deepEqual", () => {
  it("treats equal nested objects as equal regardless of key order", () => {
    const a = { x: 1, nested: { p: true, q: [1, 2, 3] } };
    const b = { nested: { q: [1, 2, 3], p: true }, x: 1 };
    expect(deepEqual(a, b)).toBe(true);
  });

  it("returns false for a differing primitive", () => {
    expect(deepEqual({ x: 1 }, { x: 2 })).toBe(false);
  });

  it("returns false for differing array length", () => {
    expect(deepEqual({ a: [1, 2] }, { a: [1, 2, 3] })).toBe(false);
  });

  it("returns false for a missing key", () => {
    expect(deepEqual({ a: 1, b: 2 }, { a: 1 })).toBe(false);
    expect(deepEqual({ a: 1 }, { a: 1, b: 2 })).toBe(false);
  });

  it("returns false for a nested edit", () => {
    const a = { nested: { value: "before" } };
    const b = { nested: { value: "after" } };
    expect(deepEqual(a, b)).toBe(false);
  });

  it("returns true after a reverted edit (value equality)", () => {
    const baseline = { nested: { value: "before" }, list: [1, 2] };
    const edited = { nested: { value: "before" }, list: [1, 2] };
    expect(deepEqual(baseline, edited)).toBe(true);
  });

  it("distinguishes null, arrays, and objects", () => {
    expect(deepEqual(null, {})).toBe(false);
    expect(deepEqual({}, null)).toBe(false);
    expect(deepEqual([], {})).toBe(false);
    expect(deepEqual({}, [])).toBe(false);
    expect(deepEqual(null, null)).toBe(true);
  });
});
