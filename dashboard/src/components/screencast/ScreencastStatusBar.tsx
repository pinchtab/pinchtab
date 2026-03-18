import { useState } from "react";
import * as api from "../../services/api";

export type ScreencastStatus = "connecting" | "streaming" | "error";

interface Props {
  tabId: string;
  status: ScreencastStatus;
  localFps: number;
  setLocalFps: (fps: number | ((prev: number) => number)) => void;
  fpsDisplay: string;
  sizeDisplay: string;
}

export default function ScreencastStatusBar({
  tabId,
  status,
  localFps,
  setLocalFps,
  fpsDisplay,
  sizeDisplay,
}: Props) {
  const [isCapturing, setIsCapturing] = useState(false);
  const [isPdfGenerating, setIsPdfGenerating] = useState(false);

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

  return (
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
  );
}
