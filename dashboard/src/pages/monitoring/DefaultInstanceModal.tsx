import { useTranslation } from "react-i18next";
import { Button, Modal } from "../../components/atoms";

interface DefaultInstanceModalProps {
  open: boolean;
  startingDefaultInstance: boolean;
  startingDefaultMode: "headed" | "headless" | null;
  prefersHeadedLaunch: boolean;
  launchError: string;
  defaultLaunchMode: "headed" | undefined;
  onClose: () => void;
  onStart: (mode: "headed" | "headless") => void;
}

export default function DefaultInstanceModal({
  open,
  startingDefaultInstance,
  startingDefaultMode,
  prefersHeadedLaunch,
  launchError,
  defaultLaunchMode,
  onClose,
  onStart,
}: DefaultInstanceModalProps) {
  const { t } = useTranslation();

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t("defaultInstance.title")}
      actions={
        <>
          <Button
            variant="secondary"
            onClick={onClose}
            disabled={startingDefaultInstance}
          >
            {t("common.cancel")}
          </Button>
          <Button
            variant={prefersHeadedLaunch ? "primary" : "secondary"}
            onClick={() => {
              void onStart("headed");
            }}
            loading={startingDefaultMode === "headed"}
            disabled={startingDefaultInstance}
          >
            {t("defaultInstance.startHeaded")}
          </Button>
          <Button
            variant={prefersHeadedLaunch ? "secondary" : "primary"}
            onClick={() => {
              void onStart("headless");
            }}
            loading={startingDefaultMode === "headless"}
            disabled={startingDefaultInstance}
          >
            {t("defaultInstance.startHeadless")}
          </Button>
        </>
      }
    >
      <p>{t("defaultInstance.description")}</p>
      {launchError && (
        <div className="mt-3 rounded border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {launchError}
        </div>
      )}
      <p className="mt-2 text-xs text-text-muted">
        {t("defaultInstance.configuredMode", {
          mode:
            defaultLaunchMode === "headed"
              ? t("defaultInstance.modeHeaded")
              : t("defaultInstance.modeHeadless"),
        })}
      </p>
    </Modal>
  );
}
