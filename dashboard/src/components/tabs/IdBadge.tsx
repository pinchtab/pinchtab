import { useState } from "react";

interface Props {
  id: string;
  copyable?: boolean;
}

export default function IdBadge({ id, copyable = true }: Props) {
  const [copied, setCopied] = useState(false);

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(id);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const shortId = id.split("_").pop()?.substring(0, 8) || id;

  if (copyable) {
    return (
      <button
        onClick={handleCopy}
        title={`Click to copy full ID: ${id}`}
        className="group flex shrink-0 items-center gap-1.5 rounded bg-bg-elevated px-1.5 py-0.5 text-[10px] font-mono text-text-muted transition-colors hover:bg-border-subtle hover:text-text-primary focus:outline-none"
      >
        <span>{shortId}</span>
        <span className="text-[8px] opacity-0 transition-opacity group-hover:opacity-100">
          {copied ? "✅" : "📋"}
        </span>
      </button>
    );
  }

  return (
    <div className="flex shrink-0 items-center gap-1.5 rounded bg-bg-elevated px-1.5 py-0.5 text-[10px] font-mono text-text-muted">
      <span>{shortId}</span>
    </div>
  );
}
