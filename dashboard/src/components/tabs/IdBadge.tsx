import { useState } from "react";

interface Props {
  id: string;
  copyable?: boolean;
  variant?: "default" | "compact";
}

export default function IdBadge({
  id,
  copyable = true,
  variant = "default",
}: Props) {
  const [copied, setCopied] = useState(false);

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(id);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const shortId = id.split("_").pop()?.substring(0, 8) || id;

  const baseStyles =
    "flex shrink-0 items-center transition-colors focus:outline-none font-mono";

  const variantStyles = {
    default:
      "rounded bg-bg-elevated px-1.5 py-0.5 text-[10px] text-text-muted hover:bg-border-subtle hover:text-text-primary",
    compact:
      "rounded bg-primary/10 px-1.5 py-0.5 text-[9px] text-primary hover:bg-primary/20",
  };

  const content = (
    <>
      <span>{variant === "compact" ? "ID" : shortId}</span>
      {copyable && variant === "default" && (
        <span className="text-[8px] opacity-0 transition-opacity group-hover:opacity-100">
          {copied ? "✅" : "📋"}
        </span>
      )}
      {copyable && variant === "compact" && copied && (
        <span className="ml-1 text-[8px]">✅</span>
      )}
    </>
  );

  if (copyable) {
    return (
      <button
        onClick={handleCopy}
        title={`Click to copy full ID: ${id}`}
        className={`group ${baseStyles} ${variantStyles[variant]}`}
      >
        {content}
      </button>
    );
  }

  return (
    <div className={`${baseStyles} ${variantStyles[variant]}`}>{content}</div>
  );
}
