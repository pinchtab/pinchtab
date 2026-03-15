interface Props {
  message: string;
  icon?: string;
  className?: string;
}

export default function EmptyView({ message, icon, className = "" }: Props) {
  return (
    <div
      className={`flex h-full flex-col items-center justify-center p-6 text-center text-sm text-text-muted ${className}`}
    >
      {icon && <div className="mb-3 text-3xl opacity-50">{icon}</div>}
      <p className="max-w-xs">{message}</p>
    </div>
  );
}
