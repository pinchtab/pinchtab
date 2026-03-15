import { useEffect, useState } from "react";
import { Button, Input, Modal } from "../atoms";
import * as api from "../../services/api";

interface Props {
  open: boolean;
  onClose: () => void;
  onCreated: (preferredProfileKey: string) => void | Promise<void>;
}

export default function CreateProfileModal({
  open,
  onClose,
  onCreated,
}: Props) {
  const [createName, setCreateName] = useState("");
  const [createUseWhen, setCreateUseWhen] = useState("");
  const [createSource, setCreateSource] = useState("");
  const [createLoading, setCreateLoading] = useState(false);

  useEffect(() => {
    if (open) {
      return;
    }

    setCreateName("");
    setCreateUseWhen("");
    setCreateSource("");
    setCreateLoading(false);
  }, [open]);

  const handleCreate = async () => {
    if (!createName.trim() || createLoading) return;

    setCreateLoading(true);
    try {
      const created = await api.createProfile({
        name: createName.trim(),
        useWhen: createUseWhen.trim() || undefined,
      });
      onClose();
      await onCreated(created.id || created.name);
    } catch (e) {
      console.error("Failed to create profile", e);
    } finally {
      setCreateLoading(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="📁 New Profile"
      wide
      actions={
        <>
          <Button
            variant="secondary"
            disabled={createLoading}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleCreate}
            disabled={!createName.trim()}
            loading={createLoading}
          >
            Create
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Input
          label="Name"
          placeholder="e.g. personal, work, scraping"
          value={createName}
          onChange={(e) => setCreateName(e.target.value)}
        />
        <Input
          label="Use this profile when (helps agents pick the right profile)"
          placeholder="e.g. I need to access Gmail for the team account"
          value={createUseWhen}
          onChange={(e) => setCreateUseWhen(e.target.value)}
        />
        <Input
          label="Import from (optional — Chrome user data path)"
          placeholder="e.g. /Users/you/Library/Application Support/Google/Chrome"
          value={createSource}
          onChange={(e) => setCreateSource(e.target.value)}
        />
      </div>
    </Modal>
  );
}
