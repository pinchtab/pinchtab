import type { ReactNode } from "react";
import { Card } from "../../components/atoms";

export function SectionCard({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <Card className="p-5">
      <div className="mb-5 border-b border-border-subtle pb-4">
        <div className="dashboard-section-label mb-2">Settings</div>
        <h3 className="text-lg font-semibold text-text-primary">{title}</h3>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-text-muted">
          {description}
        </p>
      </div>
      <div className="space-y-4">{children}</div>
    </Card>
  );
}

export function SettingRow({
  label,
  description,
  children,
}: {
  label: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 rounded-sm border border-border-subtle bg-black/10 p-4 lg:flex-row lg:items-center lg:justify-between">
      <div className="max-w-xl">
        <div className="text-sm font-medium text-text-primary">{label}</div>
        <p className="mt-1 text-xs leading-5 text-text-muted">{description}</p>
      </div>
      <div className="w-full max-w-md">{children}</div>
    </div>
  );
}
