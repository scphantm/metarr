import { useState } from 'react'

import { queryKeys, useUpdateAdmin } from '../../api/queries'
import type { AdminUser } from '../../api/types'
import { Button, Card, Row } from '../../components/Card'
import { EditableText } from '../../components/Editable'
import { SaveIndicator } from '../../components/SaveState'

/*
 * The admin account. Username and email edit in place; the password does not —
 * a credential you cannot read back is a bad fit for click-to-edit, and it
 * needs confirming against a second field before it is sent.
 */
export function AdminSection({ admin }: { admin: AdminUser }) {
  const updateAdmin = useUpdateAdmin()

  return (
    <Card
      title="Administrator"
      description="The single administrative account. These credentials are what the sign-in form checks against."
    >
      <Row label="Username">
        <EditableText
          label="Username"
          queryKey={queryKeys.config}
          value={admin.username}
          validate={(next) => (next ? null : 'Username cannot be empty')}
          onSave={(username) => updateAdmin.mutateAsync({ username })}
        />
      </Row>

      <Row label="Email">
        <EditableText
          label="Email"
          queryKey={queryKeys.config}
          value={admin.email}
          validate={(next) =>
            next.includes('@') ? null : 'Must be an email address'
          }
          onSave={(email) => updateAdmin.mutateAsync({ email })}
        />
      </Row>

      <Row
        label="Password"
        hint="Never displayed; the server stores only a salted hash"
      >
        <PasswordChanger />
      </Row>
    </Card>
  )
}

function PasswordChanger() {
  const updateAdmin = useUpdateAdmin()

  const [open, setOpen] = useState(false)
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [done, setDone] = useState(false)

  async function submit() {
    if (password.length < 8) {
      setError('Use at least 8 characters')
      return
    }
    if (password !== confirmation) {
      setError('The two entries do not match')
      return
    }

    setError(null)
    try {
      await updateAdmin.mutateAsync({ password })
      setDone(true)
      setOpen(false)
      setPassword('')
      setConfirmation('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }

  if (!open) {
    return (
      <div className="flex items-center gap-3">
        <Button onClick={() => setOpen(true)}>Change password</Button>
        {done ? (
          <SaveIndicator state="pending" />
        ) : (
          <span className="text-xs text-ink-muted">••••••••</span>
        )}
      </div>
    )
  }

  return (
    <div className="flex max-w-sm flex-col gap-2">
      <input
        type="password"
        autoFocus
        value={password}
        placeholder="New password"
        autoComplete="new-password"
        onChange={(event) => setPassword(event.target.value)}
        className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm text-ink-strong focus:border-blue"
      />
      <input
        type="password"
        value={confirmation}
        placeholder="Confirm password"
        autoComplete="new-password"
        onChange={(event) => setConfirmation(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') void submit()
          if (event.key === 'Escape') setOpen(false)
        }}
        className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm text-ink-strong focus:border-blue"
      />
      {error ? <span className="text-xs text-red">{error}</span> : null}
      <div className="flex gap-2">
        <Button variant="primary" onClick={() => void submit()}>
          Update password
        </Button>
        <Button
          variant="ghost"
          onClick={() => {
            setOpen(false)
            setError(null)
            setPassword('')
            setConfirmation('')
          }}
        >
          Cancel
        </Button>
      </div>
    </div>
  )
}
