import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
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
      title={t("createProfile.title")}
      wide
      actions={
        <>
          <Button
            variant="secondary"
            disabled={createLoading}
            onClick={onClose}
          >
            {t("common.cancel")}
          </Button>
          <Button
            variant="primary"
            onClick={handleCreate}
            disabled={!createName.trim()}
            loading={createLoading}
          >
            {t("createProfile.create")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Input
          label={t("createProfile.name")}
          placeholder={t("createProfile.namePlaceholder")}
          value={createName}
          onChange={(e) => setCreateName(e.target.value)}
        />
        <Input
          label={t("createProfile.useWhen")}
          placeholder={t("createProfile.useWhenPlaceholder")}
          value={createUseWhen}
          onChange={(e) => setCreateUseWhen(e.target.value)}
        />
        <Input
          label={t("createProfile.source")}
          placeholder={t("createProfile.sourcePlaceholder")}
          value={createSource}
          onChange={(e) => setCreateSource(e.target.value)}
        />
      </div>
    </Modal>
  );
}
