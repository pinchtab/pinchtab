import { Input } from "../components/atoms";

interface Props {
  name: string;
  useWhen: string;
  onChange: (name: string, useWhen: string) => void;
  minHeight?: string;
}

export default function ProfileBasicInfoPanel({
  name,
  useWhen,
  onChange,
  minHeight = "min-h-[180px]",
}: Props) {
  return (
    <div className="space-y-4">
      <Input
        label="Name"
        value={name}
        onChange={(e) => onChange(e.target.value, useWhen)}
      />

      <div>
        <label className="dashboard-section-title mb-1 block text-[0.68rem]">
          Use this profile when
        </label>
        <textarea
          value={useWhen}
          onChange={(e) => onChange(name, e.target.value)}
          className={`${minHeight} w-full resize-y rounded border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary`}
        />
      </div>
    </div>
  );
}
