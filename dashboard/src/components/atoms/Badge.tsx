type Variant = "default" | "success" | "warning" | "danger" | "info";

interface Props {
  variant?: Variant;
  children: React.ReactNode;
}

const variants: Record<Variant, string> = {
  default: "border border-border-subtle bg-white/[0.03] text-text-secondary",
  success: "border border-success/35 bg-success/15 text-success",
  warning: "border border-warning/35 bg-warning/15 text-warning",
  danger: "border border-destructive/35 bg-destructive/10 text-destructive",
  info: "border border-info/35 bg-info/10 text-info",
};

export default function Badge({ variant = "default", children }: Props) {
  return (
    <span
      className={`inline-flex items-center rounded-sm px-2 py-1 text-[0.68rem] font-semibold uppercase tracking-[0.08em] ${variants[variant]}`}
    >
      {children}
    </span>
  );
}
