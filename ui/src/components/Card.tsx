import type { ReactNode } from 'react'

export function Card({
  title,
  description,
  actions,
  children,
}: {
  title: string
  description?: string
  actions?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="rounded-lg border border-edge bg-surface">
      <header className="flex flex-wrap items-start justify-between gap-3 border-b border-edge px-5 py-4">
        <div>
          <h2 className="text-sm font-semibold tracking-wide text-ink-strong uppercase">
            {title}
          </h2>
          {description ? (
            <p className="mt-1 max-w-2xl text-sm text-ink-muted">
              {description}
            </p>
          ) : null}
        </div>
        {actions ? (
          <div className="flex items-center gap-2">{actions}</div>
        ) : null}
      </header>
      <div className="px-5 py-4">{children}</div>
    </section>
  )
}

// Row is the label/value pair every edit-in-place field sits in, so labels line
// up down the whole page regardless of which editor a field uses.
export function Row({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-1 border-b border-edge/60 py-3 last:border-b-0 sm:flex-row sm:items-baseline sm:gap-4">
      <div className="sm:w-52 sm:shrink-0">
        <div className="text-sm text-ink-strong">{label}</div>
        {hint ? (
          <div className="mt-0.5 text-xs text-ink-muted">{hint}</div>
        ) : null}
      </div>
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  )
}

export function Button({
  children,
  onClick,
  variant = 'default',
  type = 'button',
  disabled,
  title,
}: {
  children: ReactNode
  onClick?: () => void
  variant?: 'default' | 'primary' | 'danger' | 'ghost'
  type?: 'button' | 'submit'
  disabled?: boolean
  title?: string
}) {
  const styles: Record<string, string> = {
    default:
      'border-edge-strong/50 bg-surface-hover text-ink-strong hover:border-edge-strong',
    primary: 'border-blue bg-blue text-canvas hover:opacity-90',
    danger: 'border-red/50 text-red hover:bg-red/10',
    ghost: 'border-transparent text-ink-muted hover:text-ink-strong',
  }

  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={`rounded border px-2.5 py-1 text-xs transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${styles[variant]}`}
    >
      {children}
    </button>
  )
}

export function EmptyState({ children }: { children: ReactNode }) {
  return (
    <p className="rounded border border-dashed border-edge px-4 py-6 text-center text-sm text-ink-muted">
      {children}
    </p>
  )
}
