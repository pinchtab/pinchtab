import { useState } from "react";
import { Button } from "../atoms";
import type { Profile, Instance } from "../../generated/types";

interface Props {
  profile: Profile;
  instance?: Instance;
  onLaunch: () => void;
  onStop: () => void;
  onSave: () => void;
  onDelete: () => void;
  isSaveDisabled: boolean;
}

export default function ProfileToolbar({
  profile,
  instance,
  onLaunch,
  onStop,
  onSave,
  onDelete,
  isSaveDisabled,
}: Props) {
  const [copyFeedback, setCopyFeedback] = useState("");
  const isRunning = instance?.status === "running";

  const handleCopyId = async () => {
    if (!profile.id) return;
    try {
      await navigator.clipboard.writeText(profile.id);
      setCopyFeedback("Copied");
      setTimeout(() => setCopyFeedback(""), 2000);
    } catch {
      setCopyFeedback("Failed");
      setTimeout(() => setCopyFeedback(""), 2000);
    }
  };

  const headerMeta = [instance?.attached ? "CDP attached" : null].filter(
    Boolean,
  );

  return (
    <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-3">
          <h2 className="truncate text-lg font-semibold text-text-primary">
            {profile.name}
          </h2>
          {headerMeta.length > 0 && (
            <div className="text-sm text-text-muted">
              {headerMeta.join(" · ")}
            </div>
          )}
        </div>
        {profile.useWhen?.trim() && (
          <p className="mt-1 text-sm text-text-muted">{profile.useWhen}</p>
        )}
      </div>
      <div className="flex shrink-0 flex-wrap gap-2">
        {profile.id && (
          <Button size="sm" variant="secondary" onClick={handleCopyId}>
            {copyFeedback || "Copy ID"}
          </Button>
        )}
        <Button size="sm" variant="secondary" onClick={onDelete}>
          Delete
        </Button>
        <Button
          size="sm"
          variant="primary"
          onClick={onSave}
          disabled={isSaveDisabled}
        >
          Save
        </Button>
        {isRunning ? (
          <Button size="sm" variant="danger" onClick={onStop}>
            Stop
          </Button>
        ) : (
          <Button size="sm" variant="primary" onClick={onLaunch}>
            Start
          </Button>
        )}
      </div>
    </div>
  );
}
