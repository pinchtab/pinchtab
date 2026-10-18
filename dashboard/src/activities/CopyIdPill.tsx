import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

function compactId(value: string): string {
  if (value.length <= 12) {
    return value;
  }
  return `${value.slice(0, 4)}…${value.slice(-4)}`;
}

interface CopyIdPillProps {
  id: string;
  compact?: boolean;
  inline?: boolean;
}

export default function CopyIdPill({
  id,
  compact = false,
  inline = false,
}: CopyIdPillProps) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) {
      return;
    }
    const timeoutId = window.setTimeout(() => setCopied(false), 1200);
    return () => window.clearTimeout(timeoutId);
  }, [copied]);

  const handleClick = async () => {
    try {
      await navigator.clipboard.writeText(id);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };

  return (
    <button
      type="button"
      className={`dashboard-mono transition-all hover:text-primary ${
        inline
          ? "inline-flex items-center rounded border border-border-subtle/70 bg-white/4 px-1 py-[0.08rem] align-baseline text-[0.54rem] font-medium leading-none text-text-muted hover:border-primary/30 hover:bg-primary/8"
          : "rounded-sm border border-border-subtle bg-white/3 px-1.5 py-0.5 text-[0.62rem] font-semibold text-text-secondary hover:border-primary/30 hover:bg-primary/10"
      }`}
      onClick={() => {
        void handleClick();
      }}
      title={copied ? t("common.copiedShort") : t("common.copyTabId", { id })}
    >
      <span className="block truncate">
        {copied ? t("common.copiedShort") : compact ? compactId(id) : id}
      </span>
    </button>
  );
}
