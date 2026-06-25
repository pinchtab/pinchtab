import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RefObject } from "react";
import * as api from "../../services/api";
import { useScreencastInput } from "./useScreencastInput";

function makeCanvasRef(): RefObject<HTMLCanvasElement> {
  const canvas = document.createElement("canvas");
  canvas.width = 800;
  canvas.height = 600;
  vi.spyOn(canvas, "getBoundingClientRect").mockReturnValue({
    left: 0,
    top: 0,
    width: 400,
    height: 300,
    right: 400,
    bottom: 300,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect);
  return { current: canvas };
}

function renderInput() {
  const canvasRef = makeCanvasRef();
  const { result } = renderHook(() =>
    useScreencastInput({ canvasRef, tabId: "tab_1", status: "streaming" }),
  );
  return result;
}

describe("useScreencastInput modifiers", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("forwards the Shift modifier (bitmask 8) on click", async () => {
    const sendAction = vi.spyOn(api, "sendAction").mockResolvedValue(undefined);
    const result = renderInput();

    await result.current.handleCanvasClick({
      clientX: 100,
      clientY: 75,
      altKey: false,
      ctrlKey: false,
      metaKey: false,
      shiftKey: true,
    } as React.MouseEvent<HTMLCanvasElement>);

    expect(sendAction).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "click",
        // (100 / 400) * 800, (75 / 300) * 600 → frame-pixel coords
        x: 200,
        y: 150,
        hasXY: true,
        modifiers: 8,
      }),
    );
  });

  it("combines Cmd+Shift into the bitmask (4 | 8 = 12) on click", async () => {
    const sendAction = vi.spyOn(api, "sendAction").mockResolvedValue(undefined);
    const result = renderInput();

    await result.current.handleCanvasClick({
      clientX: 0,
      clientY: 0,
      altKey: false,
      ctrlKey: false,
      metaKey: true,
      shiftKey: true,
    } as React.MouseEvent<HTMLCanvasElement>);

    expect(sendAction).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "click", modifiers: 12 }),
    );
  });

  it("sends modifiers: 0 for a plain click", async () => {
    const sendAction = vi.spyOn(api, "sendAction").mockResolvedValue(undefined);
    const result = renderInput();

    await result.current.handleCanvasClick({
      clientX: 10,
      clientY: 10,
      altKey: false,
      ctrlKey: false,
      metaKey: false,
      shiftKey: false,
    } as React.MouseEvent<HTMLCanvasElement>);

    expect(sendAction).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "click", modifiers: 0 }),
    );
  });

  it("forwards modifiers on wheel/scroll", async () => {
    const sendAction = vi.spyOn(api, "sendAction").mockResolvedValue(undefined);
    const result = renderInput();

    await result.current.handleCanvasWheel({
      clientX: 100,
      clientY: 75,
      deltaY: 120,
      altKey: false,
      ctrlKey: true,
      metaKey: false,
      shiftKey: false,
      preventDefault: () => {},
    } as unknown as React.WheelEvent<HTMLCanvasElement>);

    expect(sendAction).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "scroll",
        scrollY: 120,
        modifiers: 2,
      }),
    );
  });
});
