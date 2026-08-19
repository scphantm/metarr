import { useState } from 'react'

import { Spinner } from '../components/SaveState'
import { useTheme } from '../theme/ThemeContext'
import { useAuth } from './AuthContext'

export function LoginScreen() {
  const { login } = useAuth()
  const { theme, toggleTheme } = useTheme()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await login({ username, password })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-full items-center justify-center bg-canvas px-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 text-center">
          <h1 className="text-2xl font-semibold tracking-tight text-ink-strong">
            Metarr
          </h1>
          <p className="mt-1 text-sm text-ink-muted">
            Sign in with the admin account
          </p>
        </div>

        <form
          onSubmit={submit}
          className="rounded-lg border border-edge bg-surface p-6"
        >
          <label className="block">
            <span className="text-xs tracking-wide text-ink-muted uppercase">
              Username
            </span>
            <input
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              autoComplete="username"
              autoFocus
              required
              className="mt-1 w-full rounded border border-edge-strong/40 bg-canvas px-3 py-2 text-sm text-ink-strong focus:border-blue"
            />
          </label>

          <label className="mt-4 block">
            <span className="text-xs tracking-wide text-ink-muted uppercase">
              Password
            </span>
            <input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="current-password"
              required
              className="mt-1 w-full rounded border border-edge-strong/40 bg-canvas px-3 py-2 text-sm text-ink-strong focus:border-blue"
            />
          </label>

          {error ? (
            <p className="mt-4 rounded border border-red/40 bg-red/10 px-3 py-2 text-sm text-red">
              {error}
            </p>
          ) : null}

          <button
            type="submit"
            disabled={submitting}
            className="mt-6 flex w-full items-center justify-center gap-2 rounded border border-blue bg-blue px-3 py-2 text-sm text-canvas transition-opacity hover:opacity-90 disabled:opacity-50"
          >
            {submitting ? <Spinner /> : null}
            {submitting ? 'Signing in…' : 'Sign in'}
          </button>
        </form>

        <button
          type="button"
          onClick={toggleTheme}
          className="mt-6 w-full text-center text-xs text-ink-muted hover:text-ink-strong"
        >
          Switch to Solarized {theme === 'dark' ? 'Light' : 'Dark'}
        </button>
      </div>
    </div>
  )
}
