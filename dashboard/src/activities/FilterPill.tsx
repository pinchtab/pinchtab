interface FilterPillProps {
  label: string;
  onClick: () => void;
}

export default function FilterPill({ label, onClick }: FilterPillProps) {
  return (
    <button
      type="button"
      className="rounded-sm border border-border-subtle bg-white/3 px-1.5 py-0.5 text-[0.62rem] font-semibold tracking-[0.08em] text-text-secondary transition-all hover:border-primary/30 hover:bg-primary/10 hover:text-primary"
      onClick={onClick}
      title={label}
    >
      <span className="block truncate">{label}</span>
    </button>
  );
}
