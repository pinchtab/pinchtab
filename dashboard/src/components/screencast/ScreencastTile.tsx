import { useRef } from "react";
import ScreencastStatusBar from "./ScreencastStatusBar";
import { useScreencastStream } from "./useScreencastStream";
import { useScreencastInput } from "./useScreencastInput";

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
  const stream = useScreencastStream({
    instanceId,
    tabId,
    quality,
    maxWidth,
    fps,
    canvasRef,
  });
  const input = useScreencastInput({
    canvasRef,
    tabId,
    status: stream.status,
  });

  const statusColor =
    stream.status === "streaming"
      ? "bg-success"
      : stream.status === "connecting"
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
        {!stream.hasFrame && stream.fallbackUrl ? (
          <img
            src={stream.fallbackUrl}
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
            onClick={input.handleCanvasClick}
            onWheel={input.handleCanvasWheel}
            onKeyDown={input.handleKeyDown}
            onKeyUp={input.handleKeyUp}
          />
        )}

        {stream.status === "error" && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-black/80 text-sm text-text-primary backdrop-blur-[2px]">
            <div className="font-semibold text-white drop-shadow-md">
              Connection lost
            </div>
            <div className="flex gap-2">
              {!stream.fallbackUrl && (
                <button
                  onClick={stream.captureFallback}
                  className="rounded bg-white/10 px-3 py-1.5 font-medium shadow-lg backdrop-blur-md transition-colors hover:bg-white/20"
                >
                  Show static preview
                </button>
              )}
              <button
                onClick={() => {
                  stream.retry();
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
        status={stream.status}
        localFps={stream.localFps}
        setLocalFps={stream.setLocalFps}
        fpsDisplay={stream.fpsDisplay}
        sizeDisplay={stream.sizeDisplay}
      />
    </div>
  );
}
