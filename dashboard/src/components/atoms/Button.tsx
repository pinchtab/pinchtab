import type { ButtonHTMLAttributes } from "react";
import { forwardRef } from "react";

type Variant = "default" | "primary" | "secondary" | "danger" | "ghost";
type Size = "sm" | "md" | "lg";

interface Props extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  loading?: boolean;
}

const base =
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-sm font-semibold transition-all duration-150 active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-50";

const variants: Record<Variant, string> = {
  default:
    "border border-border-subtle bg-white/5 text-text-primary hover:border-border-default hover:bg-white/[0.08]",
  primary:
    "bg-primary text-white shadow-[0_0_24px_rgb(var(--brand-accent-rgb)/0.18)] hover:bg-primary-hover",
  secondary:
    "border border-border-subtle bg-transparent text-text-secondary hover:border-border-default hover:bg-bg-hover hover:text-text-primary",
  danger:
    "border border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/15",
  ghost:
    "bg-transparent text-text-secondary hover:bg-bg-hover hover:text-text-primary",
};

const sizes: Record<Size, string> = {
  sm: "px-3 py-1.5 text-xs",
  md: "px-3.5 py-2 text-sm",
  lg: "px-5 py-2.5 text-base",
};

// Spinner sizes match button text sizes
const spinnerSizes: Record<Size, string> = {
  sm: "h-3 w-3",
  md: "h-4 w-4",
  lg: "h-5 w-5",
};

function Spinner({ size }: { size: Size }) {
  return (
    <svg
      className={`animate-spin ${spinnerSizes[size]}`}
      xmlns="http://www.w3.org/2000/svg"
      fill="none"
      viewBox="0 0 24 24"
    >
      <circle
        className="opacity-25"
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeWidth="4"
      />
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
      />
    </svg>
  );
}

const Button = forwardRef<HTMLButtonElement, Props>(
  (
    {
      variant = "default",
      size = "md",
      loading = false,
      className = "",
      children,
      disabled,
      ...props
    },
    ref,
  ) => {
    return (
      <button
        ref={ref}
        className={`${base} ${variants[variant]} ${sizes[size]} ${className}`}
        disabled={disabled || loading}
        {...props}
      >
        {loading && <Spinner size={size} />}
        {children}
      </button>
    );
  },
);

Button.displayName = "Button";
export default Button;
