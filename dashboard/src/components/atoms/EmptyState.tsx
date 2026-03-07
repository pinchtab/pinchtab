interface Props {
  icon?: string;
  title: string;
  description?: string;
  action?: React.ReactNode;
}

export default function EmptyState({
  icon = "🦀",
  title,
  description,
  action,
}: Props) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <div className="mb-3 flex h-[4.5rem] w-[4.5rem] items-center justify-center rounded-full border border-border-subtle bg-primary/[0.08] text-4xl shadow-[0_0_30px_rgb(var(--brand-accent-rgb)/0.08)]">
        {icon}
      </div>
      <div className="dashboard-section-label mb-2">Dashboard</div>
      <div className="text-lg font-semibold text-text-primary">{title}</div>
      {description && (
        <div className="mt-1 max-w-md text-sm leading-6 text-text-muted">
          {description}
        </div>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}
