import { useEffect } from 'react'
import Button from './Button'

interface Props {
  open: boolean
  onClose: () => void
  title: string
  children: React.ReactNode
  actions?: React.ReactNode
  wide?: boolean
}

export default function Modal({ open, onClose, title, children, actions, wide }: Props) {
  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    if (open) {
      document.addEventListener('keydown', handleEsc)
      document.body.style.overflow = 'hidden'
    }
    return () => {
      document.removeEventListener('keydown', handleEsc)
      document.body.style.overflow = ''
    }
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      onClick={onClose}
    >
      <div
        className={`max-h-[90vh] overflow-y-auto rounded-lg border border-border-subtle bg-bg-surface p-5 shadow-xl ${
          wide ? 'w-full max-w-lg' : 'w-full max-w-sm'
        }`}
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="mb-4 text-lg font-semibold text-text-primary">{title}</h3>
        <div className="text-sm text-text-secondary">{children}</div>
        <div className="mt-5 flex justify-end gap-2">
          {actions || (
            <Button variant="secondary" onClick={onClose}>
              Close
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
