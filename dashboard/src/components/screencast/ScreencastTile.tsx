import { useCallback, useEffect, useRef, useState } from "react";
import { sameOriginUrl } from "../../services/auth";
import * as api from "../../services/api";
import ScreencastStatusBar, {
  type ScreencastStatus,
} from "./ScreencastStatusBar";

interface Props {
  instanceId: string;
  tabId: string;
  label: string;
  url: string;
  quality?: number;
  maxWidth?: number;
  fps?: number;
  showTitle?: boolean;
}

function loadImageElement(blob: Blob): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const imgUrl = URL.createObjectURL(blob);
    const img = new Image();

    img.onload = () => {
      URL.revokeObjectURL(imgUrl);
      resolve(img);
    };
    img.onerror = () => {
      URL.revokeObjectURL(imgUrl);
      reject(new Error("Failed to decode screencast frame"));
    };

    img.src = imgUrl;
  });
}

async function drawFrameToCanvas(
  canvas: HTMLCanvasElement,
  ctx: CanvasRenderingContext2D,
  frameData: ArrayBuffer | Blob,
): Promise<void> {
  const blob =
    frameData instanceof Blob
      ? frameData
      : new Blob([frameData], { type: "image/jpeg" });

  if (typeof window.createImageBitmap === "function") {
    const bitmap = await window.createImageBitmap(blob);
    try {
      canvas.width = bitmap.width;
      canvas.height = bitmap.height;
      ctx.drawImage(bitmap, 0, 0);
      return;
    } finally {
      bitmap.close();
    }
  }

  const image = await loadImageElement(blob);
  canvas.width = image.width;
  canvas.height = image.height;
  ctx.drawImage(image, 0, 0);
}

export default function ScreencastTile({
  instanceId,
  tabId,
  label,
  url,
  quality = 40,
  maxWidth = 800,
  fps = 1,
  showTitle = true,
}: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState<ScreencastStatus>("connecting");
  const [fpsDisplay, setFpsDisplay] = useState("—");
  const [sizeDisplay, setSizeDisplay] = useState("—");
  const [localFps, setLocalFps] = useState(fps);
  const [hasFrame, setHasFrame] = useState(false);
  const [fallbackUrl, setFallbackUrl] = useState<string | null>(null);
  const [retryKey, setRetryKey] = useState(0);

  // ── Interactive click & scroll on the screencast canvas ──

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
      };
    },
    [],
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
  }, []);

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
  ]);

  const statusColor =
    status === "streaming"
      ? "bg-success"
      : status === "connecting"
        ? "bg-warning"
        : "bg-destructive";
  return (
    <div className="flex h-full flex-col overflow-hidden border border-border-subtle bg-bg-elevated">
      {/* Header */}
      {showTitle && (
        <div className="flex shrink-0 items-center justify-between border-b border-border-subtle px-3 py-2">
          <div className="flex items-center gap-2">
            <span className="font-mono text-xs text-text-secondary">
              {label}
            </span>
            <div className={`h-2 w-2 rounded-full ${statusColor}`} />
          </div>
          <span className="max-w-50 truncate text-xs text-text-muted">
            {url}
          </span>
        </div>
      )}
      {/* Canvas */}
      <div className="relative flex min-h-0 flex-1 items-center justify-center bg-black">
        {!hasFrame && fallbackUrl ? (
          <img
            src={fallbackUrl}
            alt="Tab preview"
            className="max-h-full max-w-full object-contain"
          />
        ) : (
          <canvas
            ref={canvasRef}
            className="max-h-full max-w-full cursor-pointer object-contain"
            tabIndex={0}
            width={800}
            height={600}
            onClick={handleCanvasClick}
            onWheel={handleCanvasWheel}
            onKeyDown={handleKeyDown}
          />
        )}

        {status === "error" && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-black/80 text-sm text-text-primary backdrop-blur-[2px]">
            <div className="font-semibold text-white drop-shadow-md">
              Connection lost
            </div>
            <div className="flex gap-2">
              {!fallbackUrl && (
                <button
                  onClick={captureFallback}
                  className="rounded bg-white/10 px-3 py-1.5 font-medium shadow-lg backdrop-blur-md transition-colors hover:bg-white/20"
                >
                  Show static preview
                </button>
              )}
              <button
                onClick={() => {
                  setStatus("connecting");
                  setHasFrame(false);
                  setRetryKey((prev) => prev + 1);
                }}
                className="rounded bg-primary/30 px-3 py-1.5 font-medium text-white shadow-lg backdrop-blur-md transition-colors hover:bg-primary/40"
              >
                Retry connection
              </button>
            </div>
          </div>
        )}
      </div>
      <ScreencastStatusBar
        tabId={tabId}
        status={status}
        localFps={localFps}
        setLocalFps={setLocalFps}
        fpsDisplay={fpsDisplay}
        sizeDisplay={sizeDisplay}
      />
    </div>
  );
}
