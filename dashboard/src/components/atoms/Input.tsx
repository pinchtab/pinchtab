import type { InputHTMLAttributes } from 'react'
import { forwardRef } from 'react'

interface Props extends InputHTMLAttributes<HTMLInputElement> {
  label?: string
  hint?: string
}

const Input = forwardRef<HTMLInputElement, Props>(
  ({ label, hint, className = '', ...props }, ref) => {
    return (
      <div className="flex flex-col gap-1">
        {label && (
          <label className="text-xs text-text-muted">{label}</label>
        )}
        <input
          ref={ref}
          className={`rounded border border-border-default bg-bg-elevated px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-primary focus:outline-none ${className}`}
          {...props}
        />
        {hint && (
          <span className="text-xs text-text-muted">{hint}</span>
        )}
      </div>
    )
  }
)

Input.displayName = 'Input'
export default Input
