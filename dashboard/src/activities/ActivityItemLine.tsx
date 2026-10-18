import {
  IconCamera,
  IconCompass,
  IconFileText,
  IconHandClick,
  IconKeyboard,
  IconMessageCircle,
  IconPointer,
  IconScreenShare,
} from "../components/atoms/Icon";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { resumeTab } from "../services/api";
import { activityStatusVariant } from "./helpers";
import type { ActivityFilters, DashboardActivityEvent } from "./types";
import CopyIdPill from "./CopyIdPill";
import FilterPill from "./FilterPill";

function formatTime(ts: string): string {
  return new Date(ts).toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function EventIcon({ event }: { event: DashboardActivityEvent }) {
  if (event.channel === "progress") return <IconMessageCircle />;
  if (event.action === "click" || event.action === "dblclick")
    return <IconHandClick />;
  if (event.action === "type") return <IconKeyboard />;
  if (event.action === "hover") return <IconPointer />;
  if (event.path.includes("/navigate")) return <IconCompass />;
  if (event.path.includes("/snapshot")) return <IconCamera />;
  if (event.path.includes("/screencast")) return <IconScreenShare />;
  return <IconFileText />;
}

function quoted(value: string): string {
  return `"${value}"`;
}

function eventDescription(event: DashboardActivityEvent, t: TFunction): string {
  if (event.channel === "progress" && event.message) {
    return event.message;
  }
  if (event.path.includes("/navigate")) {
    return event.url
      ? t("activitySummary.navigateTo", { url: event.url })
      : t("activitySummary.navigate");
  }
  if (event.path.includes("/snapshot")) {
    return t("activitySummary.snapshot");
  }
  if (event.path.includes("/screencast")) {
    return t("activitySummary.screencast");
  }
  if (event.path.includes("/text")) {
    return t("activitySummary.text");
  }
  if (event.path.includes("/screenshot")) {
    return t("activitySummary.screenshot");
  }
  if (event.path.includes("/pdf")) {
    return t("activitySummary.pdf");
  }
  switch (event.action) {
    case "click":
      return event.ref
        ? t("activitySummary.clickRef", { ref: quoted(event.ref) })
        : t("activitySummary.click");
    case "dblclick":
      return event.ref
        ? t("activitySummary.dblclickRef", { ref: quoted(event.ref) })
        : t("activitySummary.dblclick");
    case "type":
      return event.ref
        ? t("activitySummary.typeRef", { ref: quoted(event.ref) })
        : t("activitySummary.type");
    case "hover":
      return event.ref
        ? t("activitySummary.hoverRef", { ref: quoted(event.ref) })
        : t("activitySummary.hover");
    case "fill":
      return event.ref
        ? t("activitySummary.fillRef", { ref: quoted(event.ref) })
        : t("activitySummary.fill");
    case "select":
      return event.ref
        ? t("activitySummary.selectRef", { ref: quoted(event.ref) })
        : t("activitySummary.select");
    case "scroll":
      return t("activitySummary.scroll");
    case "press":
      return event.ref
        ? t("activitySummary.pressRef", { ref: quoted(event.ref) })
        : t("activitySummary.press");
    case "wait":
      return t("activitySummary.wait");
    case "evaluate":
      return t("activitySummary.evaluate");
    case "upload":
      return t("activitySummary.upload");
    case "download":
      return t("activitySummary.download");
    default:
      if (event.action) {
        return `${event.action} ${event.ref ? quoted(event.ref) : ""}`.trim();
      }
      return `${event.method} ${event.path}`;
  }
}

const statusColor: Record<string, string> = {
  success: "text-success",
  warning: "text-warning",
  danger: "text-destructive",
  default: "text-text-muted",
};

interface Props {
  event: DashboardActivityEvent;
  showTab?: boolean;
  copyTabId?: boolean;
  sessionLabel?: string;
  inHandoff?: boolean;
  onFilterChange?: (key: keyof ActivityFilters, value: string) => void;
}

export default function ActivityItemLine({
  event,
  showTab = true,
  copyTabId = false,
  sessionLabel,
  inHandoff = false,
  onFilterChange,
}: Props) {
  const { t } = useTranslation();
  const variant = activityStatusVariant(event.status);
  const [resuming, setResuming] = useState(false);
  const [resumeError, setResumeError] = useState("");

  const showResumeButton = inHandoff && Boolean(event.tabId);
  const handleResume = async () => {
    if (!event.tabId || resuming) return;
    setResuming(true);
    setResumeError("");
    try {
      await resumeTab(event.tabId);
    } catch (err) {
      setResumeError(
        err instanceof Error ? err.message : t("activityTimeline.resumeFailed"),
      );
      setResuming(false);
    }
  };

  return (
    <div className="flex items-center gap-2.5 px-4 py-2 text-sm transition-colors hover:bg-white/2">
      <span className="shrink-0 text-text-muted">
        <EventIcon event={event} />
      </span>

      <span className="dashboard-mono w-16 shrink-0 text-[0.68rem] text-text-muted">
        {formatTime(event.timestamp)}
      </span>

      {inHandoff && (
        <span
          aria-label={t("tabs.handoffDot")}
          title={t("tabs.handoffDotTitle")}
          className="inline-block h-2 w-2 shrink-0 rounded-full bg-red-500 ring-2 ring-bg-surface"
        />
      )}

      <span className="min-w-0 flex-1 truncate text-text-primary">
        {eventDescription(event, t)}
      </span>

      {showResumeButton && (
        <button
          type="button"
          onClick={handleResume}
          disabled={resuming}
          title={resumeError || t("activityTimeline.resumeTitle")}
          className="shrink-0 rounded-sm border border-warning/40 bg-warning/10 px-2 py-0.5 text-[0.68rem] text-warning transition-colors hover:bg-warning/20 disabled:opacity-50"
        >
          {resuming
            ? t("activityTimeline.resuming")
            : t("activityTimeline.resolveChallenge")}
        </button>
      )}

      {sessionLabel && (
        <span className="truncate rounded-sm border border-border-subtle bg-white/3 px-1.5 py-0.5 text-[0.68rem] text-text-muted">
          {sessionLabel}
        </span>
      )}

      {event.route && (
        <span className="flex shrink-0 items-center gap-1">
          <span className="rounded-sm bg-white/6 px-1.5 py-0.5 text-[0.68rem] text-text-muted">
            {event.route.usedProvider}
          </span>
          {event.route.escalated && (
            <span
              className="rounded-sm bg-warning/10 px-1.5 py-0.5 text-[0.68rem] text-warning"
              title={event.route.reason || t("activityTimeline.escalatedTitle")}
            >
              {"↑"} {t("activityTimeline.escalated")}
              {event.route.reason ? `: ${event.route.reason}` : ""}
            </span>
          )}
        </span>
      )}

      {showTab &&
        event.tabId &&
        onFilterChange &&
        (copyTabId ? (
          <CopyIdPill id={event.tabId} compact />
        ) : (
          <FilterPill
            label={`tab:${event.tabId}`}
            onClick={() => onFilterChange("tabId", event.tabId || "")}
          />
        ))}

      <span
        className={`dashboard-mono shrink-0 text-[0.68rem] ${statusColor[variant] || "text-text-muted"}`}
      >
        {event.status}
      </span>
    </div>
  );
}
