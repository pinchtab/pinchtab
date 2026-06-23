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
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Start Default Instance"
      actions={
        <>
          <Button
            variant="secondary"
            onClick={onClose}
            disabled={startingDefaultInstance}
          >
            Cancel
          </Button>
          <Button
            variant={prefersHeadedLaunch ? "primary" : "secondary"}
            onClick={() => {
              void onStart("headed");
            }}
            loading={startingDefaultMode === "headed"}
            disabled={startingDefaultInstance}
          >
            Start Headed
          </Button>
          <Button
            variant={prefersHeadedLaunch ? "secondary" : "primary"}
            onClick={() => {
              void onStart("headless");
            }}
            loading={startingDefaultMode === "headless"}
            disabled={startingDefaultInstance}
          >
            Start Headless
          </Button>
        </>
      }
    >
      <p>Choose how to launch the default profile for this session.</p>
      {launchError && (
        <div className="mt-3 rounded border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {launchError}
        </div>
      )}
      <p className="mt-2 text-xs text-text-muted">
        Configured default mode:{" "}
        {defaultLaunchMode === "headed" ? "headed" : "headless"}
      </p>
    </Modal>
  );
}
