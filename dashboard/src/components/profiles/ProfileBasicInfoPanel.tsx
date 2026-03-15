import { useState, useEffect } from "react";
import { Input } from "../atoms";
import type { Profile } from "../../generated/types";

interface Props {
  profile: Profile;
  onChange: (name: string, useWhen: string) => void;
  minHeight?: string;
}

export default function ProfileBasicInfoPanel({
  profile,
  onChange,
  minHeight = "min-h-[180px]",
}: Props) {
  const [name, setName] = useState(profile.name);
  const [useWhen, setUseWhen] = useState(profile.useWhen || "");

  useEffect(() => {
    setName(profile.name);
    setUseWhen(profile.useWhen || "");
  }, [profile]);

  useEffect(() => {
    onChange(name, useWhen);
  }, [name, useWhen, onChange]);

  return (
    <div className="space-y-4">
      <Input
        label="Name"
        value={name}
        onChange={(e) => setName(e.target.value)}
      />

      <div>
        <label className="dashboard-section-title mb-1 block text-[0.68rem]">
          Use this profile when
        </label>
        <textarea
          value={useWhen}
          onChange={(e) => setUseWhen(e.target.value)}
          className={`${minHeight} w-full resize-y rounded border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary`}
        />
      </div>
    </div>
  );
}
