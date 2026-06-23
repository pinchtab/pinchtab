import { useCallback, useEffect, useRef, useState } from "react";
import type { RefObject } from "react";
import { sameOriginUrl } from "../../services/auth";
import * as api from "../../services/api";
import { type ScreencastStatus } from "./ScreencastStatusBar";
import { drawFrameToCanvas } from "./frameDecode";

interface UseScreencastStreamArgs {
  instanceId: string;
  tabId: string;
  quality: number;
  maxWidth: number;
  fps: number;
  canvasRef: RefObject<HTMLCanvasElement | null>;
}

export function useScreencastStream({
  instanceId,
  tabId,
  quality,
  maxWidth,
  fps,
  canvasRef,
}: UseScreencastStreamArgs) {
  const socketRef = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState<ScreencastStatus>("connecting");
  const [fpsDisplay, setFpsDisplay] = useState("—");
  const [sizeDisplay, setSizeDisplay] = useState("—");
  const [localFps, setLocalFps] = useState(fps);
  const [hasFrame, setHasFrame] = useState(false);
  const [fallbackUrl, setFallbackUrl] = useState<string | null>(null);
  const [retryKey, setRetryKey] = useState(0);

  // Reset local FPS when the tab changes to match the new tab's initial request
  useEffect(() => {
    setLocalFps(fps);
    setStatus("connecting");
    setHasFrame(false);
    setFallbackUrl(null);
  }, [tabId, fps]);

  // Clean up static preview URL on unmount or tab change
  useEffect(() => {
    return () => {
      if (fallbackUrl) {
        URL.revokeObjectURL(fallbackUrl);
      }
    };
  }, [fallbackUrl]);

  const captureFallback = useCallback(async () => {
    try {
      const blob = await api.fetchTabScreenshot(tabId, "png");
      const nextUrl = URL.createObjectURL(blob);
      setFallbackUrl((prev) => {
        if (prev) {
          URL.revokeObjectURL(prev);
        }
        return nextUrl;
      });
    } catch (e) {
      console.error("Fallback capture failed", e);
    }
  }, [tabId]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const params = new URLSearchParams({
      tabId,
      quality: String(quality),
      maxWidth: String(maxWidth),
      fps: String(localFps),
    });
    const path = sameOriginUrl(
      `/instances/${encodeURIComponent(instanceId)}/proxy/screencast?${params.toString()}`,
    );
    const wsUrl = new URL(path, window.location.origin);
    wsUrl.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";

    const socket = new WebSocket(wsUrl.toString());
    socket.binaryType = "arraybuffer";
    socketRef.current = socket;
    setStatus("connecting");
    setHasFrame(false);

    let disposed = false;
    let hasRenderedFrame = false;
    let pendingFrame: ArrayBuffer | Blob | null = null;
    let renderInFlight = false;

    let frameCount = 0;
    let lastFpsTime = Date.now();
    const fallbackTimer = window.setTimeout(() => {
      if (!disposed && !hasRenderedFrame) {
        void captureFallback();
      }
    }, 1500);

    const renderNextFrame = async () => {
      if (disposed || renderInFlight || !pendingFrame) return;

      renderInFlight = true;
      const frameData = pendingFrame;
      pendingFrame = null;

      try {
        await drawFrameToCanvas(canvas, ctx, frameData);
        if (disposed) return;

        hasRenderedFrame = true;
        window.clearTimeout(fallbackTimer);
        setHasFrame(true);
        setStatus("streaming");
      } catch (err) {
        if (!disposed) {
          console.error("screencast frame render failed", err);
        }
      } finally {
        renderInFlight = false;
        if (!disposed && pendingFrame) {
          void renderNextFrame();
        }
      }
    };

    socket.onopen = () => {
      if (!disposed) {
        setStatus("connecting");
      }
    };

    socket.onmessage = (evt) => {
      if (disposed) return;
      pendingFrame = evt.data;
      void renderNextFrame();

      frameCount++;
      const now = Date.now();
      if (now - lastFpsTime >= 1000) {
        setFpsDisplay(`${frameCount} fps`);
        const frameBytes =
          evt.data instanceof Blob ? evt.data.size : evt.data.byteLength;
        setSizeDisplay(`${(frameBytes / 1024).toFixed(0)} KB/frame`);
        frameCount = 0;
        lastFpsTime = now;
      }
    };

    socket.onerror = () => {
      if (!disposed) {
        setStatus("error");
        void api.handleRealtimeAuthFailure();
      }
    };

    socket.onclose = () => {
      if (!disposed) {
        setStatus("error");
        void api.handleRealtimeAuthFailure();
      }
    };

    return () => {
      disposed = true;
      window.clearTimeout(fallbackTimer);
      socket.close();
      socketRef.current = null;
    };
  }, [
    instanceId,
    tabId,
    quality,
    maxWidth,
    localFps,
    retryKey,
    captureFallback,
    canvasRef,
  ]);

  const retry = () => {
    setStatus("connecting");
    setHasFrame(false);
    setRetryKey((p) => p + 1);
  };

  return {
    status,
    fpsDisplay,
    sizeDisplay,
    hasFrame,
    fallbackUrl,
    captureFallback,
    localFps,
    setLocalFps,
    retry,
  };
}
