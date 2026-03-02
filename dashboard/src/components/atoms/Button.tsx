import type { ButtonHTMLAttributes } from 'react'
import { forwardRef } from 'react'

type Variant = 'default' | 'primary' | 'secondary' | 'danger' | 'ghost'
type Size = 'sm' | 'md' | 'lg'

interface Props extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
}

const base =
  'inline-flex items-center justify-center gap-1.5 rounded font-medium transition-all duration-150 disabled:cursor-not-allowed disabled:opacity-40'

const variants: Record<Variant, string> = {
  default:
    'border border-border-default bg-bg-elevated text-text-secondary hover:bg-bg-hover hover:text-text-primary',
  primary: 'bg-primary text-white hover:bg-primary-hover',
  secondary: 'border border-border-subtle bg-transparent text-text-muted hover:bg-bg-elevated hover:text-text-secondary',
  danger: 'bg-destructive text-white hover:bg-destructive/80',
  ghost: 'bg-transparent text-text-muted hover:bg-bg-elevated hover:text-text-secondary',
}

const sizes: Record<Size, string> = {
  sm: 'px-2 py-1 text-xs',
  md: 'px-3 py-1.5 text-sm',
  lg: 'px-4 py-2 text-base',
}

const Button = forwardRef<HTMLButtonElement, Props>(
  ({ variant = 'default', size = 'md', className = '', children, ...props }, ref) => {
    return (
      <button
        ref={ref}
        className={`${base} ${variants[variant]} ${sizes[size]} ${className}`}
        {...props}
      >
        {children}
      </button>
    )
  }
)

Button.displayName = 'Button'
export default Button
