import { useEffect, useRef, useState } from "react";
import { addTokenToUrl } from "../../services/auth";

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

  // Reset local FPS when the tab changes to match the new tab's initial request
  useEffect(() => {
    setLocalFps(fps);
    setStatus("connecting");
  }, [tabId, fps]);

  const takeScreenshot = async () => {
    if (isCapturing) return;
    setIsCapturing(true);

    try {
      const params = new URLSearchParams({
        tabId,
        quality: "100",
        maxWidth: "0", // 0 means original size in backend
        fps: "1",
      });
      const path = addTokenToUrl(
        `/instances/${encodeURIComponent(instanceId)}/proxy/screencast?${params.toString()}`,
      );
      const wsUrl = new URL(path, window.location.origin);
      wsUrl.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";

      const socket = new WebSocket(wsUrl.toString());
      socket.binaryType = "arraybuffer";

      socket.onmessage = (evt) => {
        const blob = new Blob([evt.data], { type: "image/jpeg" });
        const url = URL.createObjectURL(blob);
        const link = document.createElement("a");
        link.href = url;
        link.download = `screenshot-${tabId}-${Date.now()}.jpg`;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        URL.revokeObjectURL(url);
        socket.close();
        setIsCapturing(false);
      };

      socket.onerror = () => {
        console.error("Screenshot capture failed");
        setIsCapturing(false);
      };
    } catch (e) {
      console.error("Screenshot error", e);
      setIsCapturing(false);
    }
  };

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

    let frameCount = 0;
    let lastFpsTime = Date.now();

    socket.onopen = () => {
      setStatus("streaming");
    };

    socket.onmessage = (evt) => {
      const blob = new Blob([evt.data], { type: "image/jpeg" });
      const imgUrl = URL.createObjectURL(blob);
      const img = new Image();
      img.onload = () => {
        canvas.width = img.width;
        canvas.height = img.height;
        ctx.drawImage(img, 0, 0);
        URL.revokeObjectURL(imgUrl);
      };
      img.src = imgUrl;

      frameCount++;
      const now = Date.now();
      if (now - lastFpsTime >= 1000) {
        setFpsDisplay(`${frameCount} fps`);
        setSizeDisplay(`${(evt.data.byteLength / 1024).toFixed(0)} KB/frame`);
        frameCount = 0;
        lastFpsTime = now;
      }
    };

    socket.onerror = () => {
      setStatus("error");
    };

    socket.onclose = () => {
      setStatus("error");
    };

    return () => {
      socket.close();
      socketRef.current = null;
    };
  }, [instanceId, tabId, quality, maxWidth, localFps]);

  const statusColor =
    status === "streaming" ? "bg-success" : status === "connecting";
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
        <canvas
          ref={canvasRef}
          className="max-h-full max-w-full object-contain"
          width={800}
          height={600}
        />
        {status === "error" && (
          <div className="absolute inset-0 flex items-center justify-center bg-black/80 text-sm text-text-muted">
            Connection lost
          </div>
        )}
      </div>
      {/* Footer */}
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
            <div className="min-w-[4rem] px-1.5 text-center font-mono text-[10px] text-text-secondary">
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
            title="Take full quality screenshot"
          >
            {isCapturing ? (
              <span className="animate-pulse">⌛</span>
            ) : (
              <span>📸</span>
            )}
          </button>
        </div>
        <span>{sizeDisplay}</span>
      </div>{" "}
    </div>
  );
}
