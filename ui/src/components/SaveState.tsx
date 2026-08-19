import type { SaveState } from './useSaveState'

/*
 * The visual vocabulary for the save lifecycle. It is deliberately small and
 * always in the same place, so a user learns it once: a spinner means in
 * flight, a hollow dot means accepted but not yet stored, a tick means the
 * server has confirmed it, and anything red needs reading.
 */

export function SaveIndicator({
  state,
  error,
  onDismissError,
}: {
  state: SaveState
  error?: string | null
  onDismissError?: () => void
}) {
  if (state === 'idle') {
    return null
  }

  if (state === 'error') {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs text-red">
        <span aria-hidden="true">✕</span>
        <span>{error ?? 'Save failed'}</span>
        {onDismissError ? (
          <button
            type="button"
            onClick={onDismissError}
            className="underline underline-offset-2 hover:text-ink-strong"
          >
            dismiss
          </button>
        ) : null}
      </span>
    )
  }

  if (state === 'saving') {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs text-ink-muted">
        <Spinner />
        <span>Sending…</span>
      </span>
    )
  }

  if (state === 'pending') {
    return (
      <span
        className="inline-flex items-center gap-1.5 text-xs text-yellow"
        title="The API accepted this write and queued it. It is stored once the background listener has processed the event."
      >
        <span aria-hidden="true">◌</span>
        <span>Queued</span>
      </span>
    )
  }

  if (state === 'unconfirmed') {
    return (
      <span
        className="inline-flex items-center gap-1.5 text-xs text-orange"
        title="The write was accepted but the server has not reported the new value yet. It may still land; reload to check."
      >
        <span aria-hidden="true">!</span>
        <span>Not confirmed</span>
      </span>
    )
  }

  return (
    <span className="inline-flex items-center gap-1.5 text-xs text-green">
      <span aria-hidden="true">✓</span>
      <span>Saved</span>
    </span>
  )
}

export function Spinner() {
  return (
    <span
      aria-hidden="true"
      className="inline-block h-3 w-3 animate-spin rounded-full border border-ink-muted border-t-transparent"
    />
  )
}
