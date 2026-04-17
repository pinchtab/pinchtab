import type { ReactNode, SelectHTMLAttributes } from "react";
import { forwardRef, useId } from "react";

interface Props extends SelectHTMLAttributes<HTMLSelectElement> {
  children?: ReactNode;
  label?: string;
  hint?: string;
  variant?: "default" | "compact";
}

const baseClassName =
  "appearance-none rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.72)] bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20viewBox%3D%220%200%2020%2020%22%20fill%3D%22%236b7280%22%3E%3Cpath%20fill-rule%3D%22evenodd%22%20d%3D%22M5.23%207.21a.75.75%200%20011.06.02L10%2011.168l3.71-3.938a.75.75%200%20111.08%201.04l-4.25%204.5a.75.75%200%2001-1.08%200l-4.25-4.5a.75.75%200%2001.02-1.06z%22%20clip-rule%3D%22evenodd%22%2F%3E%3C%2Fsvg%3E')] bg-[length:1.25rem_1.25rem] bg-[position:right_0.5rem_center] bg-no-repeat text-text-primary transition-all duration-150 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-40";

const variantClassName: Record<NonNullable<Props["variant"]>, string> = {
  default: "w-full px-3 py-2 pr-8 text-sm",
  compact: "px-2 py-1 pr-7 text-xs",
};

const Select = forwardRef<HTMLSelectElement, Props>(
  ({ label, hint, variant = "default", className = "", ...props }, ref) => {
    const generatedId = useId();
    const selectId = props.id || generatedId;
    const select = (
      <select
        id={selectId}
        ref={ref}
        className={`${baseClassName} ${variantClassName[variant]} ${className}`}
        {...props}
      />
    );

    if (!label && !hint) {
      return select;
    }

    return (
      <div className="flex flex-col gap-1.5">
        {label && (
          <label
            htmlFor={selectId}
            className="dashboard-section-title text-[0.68rem]"
          >
            {label}
          </label>
        )}
        {select}
        {hint && <span className="text-xs text-text-muted">{hint}</span>}
      </div>
    );
  },
);

Select.displayName = "Select";

export default Select;
