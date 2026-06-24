import { useCallback, useEffect } from "react";
import type { RefObject } from "react";
import * as api from "../../services/api";
import { type ScreencastStatus } from "./ScreencastStatusBar";

interface UseScreencastInputArgs {
  canvasRef: RefObject<HTMLCanvasElement | null>;
  tabId: string;
  status: ScreencastStatus;
}

export function useScreencastInput({
  canvasRef,
  tabId,
  status,
}: UseScreencastInputArgs) {
  const getPageCoords = useCallback(
    (e: React.MouseEvent<HTMLCanvasElement>) => {
      const canvas = canvasRef.current;
      if (!canvas) return null;
      const rect = canvas.getBoundingClientRect();
      // canvas.width/height = actual CDP viewport pixels
      // rect.width/height = CSS display size
      return {
        x: ((e.clientX - rect.left) / rect.width) * canvas.width,
        y: ((e.clientY - rect.top) / rect.height) * canvas.height,
        // Tell the server which frame size these coords are in, so it can scale
        // them into the live CSS viewport (the frame is larger on HiDPI).
        frameW: canvas.width,
        frameH: canvas.height,
      };
    },
    [canvasRef],
  );

  const handleCanvasClick = useCallback(
    async (e: React.MouseEvent<HTMLCanvasElement>) => {
      if (status !== "streaming") return;
      const coords = getPageCoords(e);
      if (!coords) return;
      try {
        await api.sendAction({
          kind: "click",
          tabId,
          x: Math.round(coords.x),
          y: Math.round(coords.y),
          hasXY: true,
          frameW: coords.frameW,
          frameH: coords.frameH,
        });
      } catch (err) {
        console.error("click failed", err);
      }
    },
    [status, tabId, getPageCoords],
  );

  const handleCanvasWheel = useCallback(
    async (e: React.WheelEvent<HTMLCanvasElement>) => {
      if (status !== "streaming") return;
      const coords = getPageCoords(e);
      if (!coords) return;
      try {
        await api.sendAction({
          kind: "scroll",
          tabId,
          x: Math.round(coords.x),
          y: Math.round(coords.y),
          scrollY: Math.round(e.deltaY),
          frameW: coords.frameW,
          frameH: coords.frameH,
        });
      } catch (err) {
        console.error("scroll failed", err);
      }
    },
    [status, tabId, getPageCoords],
  );

  const handleKeyDown = useCallback(
    async (e: React.KeyboardEvent<HTMLCanvasElement>) => {
      if (status !== "streaming") return;
      if ((e.ctrlKey || e.metaKey) && e.key === "v") {
        e.preventDefault();
        navigator.clipboard
          .readText()
          .then((text) => {
            if (text) {
              api
                .sendAction({ kind: "keyboard-inserttext", tabId, text })
                .catch(() => {});
            }
          })
          .catch(() => {});
        return;
      }
      e.preventDefault();
      try {
        if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
          await api.sendAction({
            kind: "keyboard-inserttext",
            tabId,
            text: e.key,
          });
        } else {
          await api.sendAction({ kind: "press", tabId, key: e.key });
        }
      } catch (err) {
        console.error("key input failed", err);
      }
    },
    [status, tabId],
  );

  // Prevent default wheel behavior on the canvas so the page doesn't scroll
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const prevent = (e: WheelEvent) => e.preventDefault();
    canvas.addEventListener("wheel", prevent, { passive: false });
    return () => canvas.removeEventListener("wheel", prevent);
  }, [canvasRef]);

  return { handleCanvasClick, handleCanvasWheel, handleKeyDown };
}
