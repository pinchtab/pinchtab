import { useCallback, useEffect, useRef, useState } from "react";
import { addTokenToUrl } from "../../services/auth";
import * as api from "../../services/api";

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

type Status = "connecting" | "streaming" | "error";

export default function ScreencastTile({
  instanceId,
  tabId,
  label,
  url,
  quality = 30,
  maxWidth = 800,
  fps = 1,
  showTitle = true,
}: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState<Status>("connecting");
  const [fpsDisplay, setFpsDisplay] = useState("—");
  const [sizeDisplay, setSizeDisplay] = useState("—");
  const [localFps, setLocalFps] = useState(fps);
  const [isCapturing, setIsCapturing] = useState(false);
  const [isPdfGenerating, setIsPdfGenerating] = useState(false);
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

  const takeScreenshot = async () => {
    if (isCapturing) return;
    setIsCapturing(true);

    try {
      const blob = await api.fetchTabScreenshot(tabId, "png");
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `screenshot-${tabId}-${Date.now()}.png`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    } catch (e) {
      console.error("Screenshot capture failed", e);
    } finally {
      setIsCapturing(false);
    }
  };

  const downloadPdf = async () => {
    if (isPdfGenerating) return;
    setIsPdfGenerating(true);

    try {
      const blob = await api.fetchTabPdf(tabId);
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `page-${tabId}-${Date.now()}.pdf`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    } catch (e) {
      console.error("PDF generation failed", e);
    } finally {
      setIsPdfGenerating(false);
    }
  };

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
    const path = addTokenToUrl(
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
    let fallbackCaptured = false;

    let frameCount = 0;
    let lastFpsTime = Date.now();
    const fallbackTimer = window.setTimeout(() => {
      if (!disposed && !fallbackCaptured) {
        void captureFallback();
      }
    }, 1500);

    socket.onopen = () => {
      if (!disposed) {
        setStatus("connecting");
      }
    };

    socket.onmessage = (evt) => {
      if (disposed) return;
      fallbackCaptured = true;
      window.clearTimeout(fallbackTimer);

      const blob = new Blob([evt.data], { type: "image/jpeg" });
      const imgUrl = URL.createObjectURL(blob);
      const img = new Image();
      img.onload = () => {
        if (disposed) {
          URL.revokeObjectURL(imgUrl);
          return;
        }
        canvas.width = img.width;
        canvas.height = img.height;
        ctx.drawImage(img, 0, 0);
        URL.revokeObjectURL(imgUrl);
      };
      img.src = imgUrl;
      setHasFrame(true);
      setStatus("streaming");

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
      }
    };

    socket.onclose = () => {
      if (!disposed) {
        setStatus("error");
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
    <div className="flex h-full flex-col overflow-hidden rounded-lg border border-border-subtle bg-bg-elevated">
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
            className="max-h-full max-w-full object-contain"
            width={800}
            height={600}
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
      <div className="flex shrink-0 items-center justify-between border-t border-border-subtle px-3 py-1 text-xs text-text-muted">
        <div className="flex items-center gap-3">
          <div className="flex items-center overflow-hidden rounded border border-border-subtle bg-black/20">
            <button
              onClick={() => setLocalFps((prev) => Math.max(1, prev - 1))}
              className="flex h-5 w-5 items-center justify-center hover:bg-white/5 active:bg-white/10"
              title="Decrease FPS"
            >
              -
            </button>
            <div className="min-w-16 px-1.5 text-center font-mono text-[10px] text-text-secondary">
              {localFps} FPS ({fpsDisplay})
            </div>
            <button
              onClick={() => setLocalFps((prev) => Math.min(30, prev + 1))}
              className="flex h-5 w-5 items-center justify-center hover:bg-white/5 active:bg-white/10"
              title="Increase FPS"
            >
              +
            </button>
          </div>

          <button
            onClick={takeScreenshot}
            disabled={isCapturing || status !== "streaming"}
            className={`flex h-6 w-6 items-center justify-center rounded-md border border-border-subtle transition-colors hover:bg-white/5 disabled:opacity-50 ${
              isCapturing ? "bg-primary/20" : "bg-black/20"
            }`}
            title="Take full quality screenshot (PNG)"
          >
            {isCapturing ? (
              <span className="animate-pulse">⌛</span>
            ) : (
              <span>📸</span>
            )}
          </button>

          <button
            onClick={downloadPdf}
            disabled={isPdfGenerating || status !== "streaming"}
            className={`flex h-6 w-6 items-center justify-center rounded-md border border-border-subtle transition-colors hover:bg-white/5 disabled:opacity-50 ${
              isPdfGenerating ? "bg-primary/20" : "bg-black/20"
            }`}
            title="Download as PDF"
          >
            {isPdfGenerating ? (
              <span className="animate-pulse">⌛</span>
            ) : (
              <span>📄</span>
            )}
          </button>
        </div>
        <span>{sizeDisplay}</span>
      </div>
    </div>
  );
}
