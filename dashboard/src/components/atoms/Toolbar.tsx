import Button from "./Button";

interface ToolbarAction {
  key: string;
  label: string;
  onClick: () => void;
  variant?: "default" | "primary";
  disabled?: boolean;
}

interface Props {
  actions?: ToolbarAction[];
  children?: React.ReactNode;
}

export default function Toolbar({ actions, children }: Props) {
  return (
    <div className="flex items-center gap-2 border-b border-border-subtle bg-bg-surface/95 px-4 py-3 backdrop-blur">
      {children}
      <div className="ml-auto flex items-center gap-2">
        {actions?.map((a) => (
          <Button
            key={a.key}
            onClick={a.onClick}
            disabled={a.disabled}
            variant={a.variant === "primary" ? "primary" : "secondary"}
            size="md"
          >
            {a.label}
          </Button>
        ))}
      </div>
    </div>
  );
}
