import type { ReactNode } from 'react'

import { useAuth } from '../auth/AuthContext'
import { useTheme } from '../theme/ThemeContext'

/*
 * The right column. Pages fill it through the SidebarContent slot; what lives
 * here permanently is the session and the theme switch.
 */

export function Sidebar({ children }: { children?: ReactNode }) {
  const { theme, toggleTheme } = useTheme()
  const { username, expiresAt, logout } = useAuth()

  return (
    <aside
      className="flex h-full flex-col gap-6 overflow-y-auto p-4"
      aria-label="Sidebar"
    >
      <section>
        <h2 className="mb-2 text-xs font-semibold tracking-wide text-ink-muted uppercase">
          Session
        </h2>
        <div className="rounded border border-edge bg-surface px-3 py-2.5 text-sm">
          <div className="text-ink-strong">{username ?? 'Signed in'}</div>
          {expiresAt ? (
            <div className="mt-0.5 text-xs text-ink-muted">
              Expires {new Date(expiresAt).toLocaleTimeString()}
            </div>
          ) : null}
          <button
            type="button"
            onClick={() => void logout()}
            className="mt-2 text-xs text-ink-muted underline underline-offset-2 hover:text-red"
          >
            Sign out
          </button>
        </div>
      </section>

      <section>
        <h2 className="mb-2 text-xs font-semibold tracking-wide text-ink-muted uppercase">
          Appearance
        </h2>
        <div className="flex gap-1 rounded border border-edge bg-surface p-1">
          {(['dark', 'light'] as const).map((option) => (
            <button
              key={option}
              type="button"
              onClick={() => option !== theme && toggleTheme()}
              className={`flex-1 rounded px-2 py-1.5 text-xs capitalize transition-colors ${
                theme === option
                  ? 'bg-surface-hover text-ink-strong'
                  : 'text-ink-muted hover:text-ink-strong'
              }`}
            >
              {option}
            </button>
          ))}
        </div>
        <p className="mt-1.5 text-xs text-ink-muted">Solarized</p>
      </section>

      {children}
    </aside>
  )
}
