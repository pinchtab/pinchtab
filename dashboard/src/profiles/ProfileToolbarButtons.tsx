import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, Modal } from "../components/atoms";
import type { Profile, Instance } from "../generated/types";

interface Props {
  profile: Profile;
  instance?: Instance;
  onLaunch: () => void;
  onStop: () => void;
  onSave: () => void;
  onDelete: () => void;
  deleteError?: string | null;
  deleteNotice?: string | null;
  isSaveDisabled: boolean;
}

export default function ProfileToolbarButtons({
  profile,
  instance,
  onLaunch,
  onStop,
  onSave,
  onDelete,
  deleteError,
  deleteNotice,
  isSaveDisabled,
}: Props) {
  const { t } = useTranslation();
  const [copyFeedback, setCopyFeedback] = useState("");
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const isRunning = instance?.status === "running";
  // Two sources ORed, not tried in order: the server's own running flag and the
  // instance join each know holders the other can miss, and this predicate only
  // hides a destructive control — a false "running" costs a hidden button, a
  // false "idle" costs the live profile. It fails SAFE: only a positive true
  // withholds Delete; an absent or false flag keeps the confirmed path.
  const deleteWithheld = profile.running === true || isRunning;

  const handleCopyId = async () => {
    if (!profile.id) return;
    try {
      await navigator.clipboard.writeText(profile.id);
      setCopyFeedback(t("profiles.copied"));
      setTimeout(() => setCopyFeedback(""), 2000);
    } catch {
      setCopyFeedback(t("profiles.copyFailed"));
      setTimeout(() => setCopyFeedback(""), 2000);
    }
  };

  const confirmDelete = () => {
    setConfirmingDelete(false);
    onDelete();
  };

  return (
    <div className="flex shrink-0 items-center gap-1.5">
      {deleteError && (
        <span role="alert" className="text-xs text-destructive">
          {deleteError}
        </span>
      )}
      {deleteNotice && (
        <span role="status" className="text-xs text-text-secondary">
          {deleteNotice}
        </span>
      )}
      {profile.id && (
        <Button size="sm" variant="secondary" onClick={handleCopyId}>
          {copyFeedback || t("profiles.copyId")}
        </Button>
      )}
      {!deleteWithheld && (
        <Button
          size="sm"
          variant="secondary"
          onClick={() => setConfirmingDelete(true)}
        >
          {t("profiles.delete")}
        </Button>
      )}
      <Button
        size="sm"
        variant="primary"
        onClick={onSave}
        disabled={isSaveDisabled}
      >
        {t("common.save")}
      </Button>
      {isRunning ? (
        <Button size="sm" variant="danger" onClick={onStop}>
          {t("profiles.stop")}
        </Button>
      ) : (
        <Button size="sm" variant="primary" onClick={onLaunch}>
          {t("profiles.start")}
        </Button>
      )}
      <Modal
        open={confirmingDelete}
        onClose={() => setConfirmingDelete(false)}
        title={t("profiles.deleteTitle")}
        actions={
          <>
            <Button
              variant="secondary"
              onClick={() => setConfirmingDelete(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button variant="danger" onClick={confirmDelete}>
              {t("profiles.deleteTitle")}
            </Button>
          </>
        }
      >
        <p>{t("profiles.deleteConfirm", { name: profile.name })}</p>
      </Modal>
    </div>
  );
}
