import type { HTMLAttributes } from 'react'

interface Props extends HTMLAttributes<HTMLDivElement> {
  hover?: boolean
  selected?: boolean
}

export default function Card({
  hover = false,
  selected = false,
  className = '',
  children,
  ...props
}: Props) {
  return (
    <div
      className={`rounded-lg border bg-bg-surface ${
        selected
          ? 'border-primary'
          : 'border-border-subtle'
      } ${
        hover
          ? 'cursor-pointer transition-all duration-150 hover:border-border-default hover:bg-bg-elevated'
          : ''
      } ${className}`}
      {...props}
    >
      {children}
    </div>
  )
}
