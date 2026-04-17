import { useState } from "react";
import { Button } from "../components/atoms";
import type { Profile, Instance } from "../generated/types";

interface Props {
  profile: Profile;
  instance?: Instance;
  onLaunch: () => void;
  onStop: () => void;
  onSave: () => void;
  onDelete: () => void;
  isSaveDisabled: boolean;
}

export default function ProfileToolbarButtons({
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

  return (
    <div className="flex shrink-0 items-center gap-1.5">
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
  );
}
