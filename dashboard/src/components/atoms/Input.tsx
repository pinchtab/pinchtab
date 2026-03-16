import type { InputHTMLAttributes } from "react";
import { forwardRef, useId } from "react";

interface Props extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  hint?: string;
}

const Input = forwardRef<HTMLInputElement, Props>(
  ({ label, hint, className = "", ...props }, ref) => {
    const generatedId = useId();
    const inputId = props.id || generatedId;

    return (
      <div className="flex flex-col gap-1.5">
        {label && (
          <label
            htmlFor={inputId}
            className="dashboard-section-title text-[0.68rem]"
          >
            {label}
          </label>
        )}
        <input
          id={inputId}
          ref={ref}
          className={`rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.72)] px-3 py-2 text-sm text-text-primary placeholder:text-text-muted transition-all duration-150 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20 ${className}`}
          {...props}
        />
        {hint && <span className="text-xs text-text-muted">{hint}</span>}
      </div>
    );
  },
);

Input.displayName = "Input";
export default Input;
