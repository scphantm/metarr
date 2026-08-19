import { useEffect, useRef, useState } from 'react'

import { SaveIndicator } from './SaveState'
import { useSaveState } from './useSaveState'

/*
 * Edit in place: a value reads as text until you click it, becomes an input,
 * and commits on Enter or blur. Escape always abandons the edit — that is the
 * contract that makes clicking a value safe to do out of curiosity.
 *
 * Nothing here knows how to save. Each field is handed an onSave that performs
 * the write, and useSaveState owns what happens between accepting it and the
 * server confirming it.
 */

// No width here on purpose: each field sets its own. Putting w-full in the
// shared base and overriding it per field does not work — both are width
// utilities, and which one wins depends on their order in the generated
// stylesheet rather than in the class string, so a narrow field would silently
// render full width and jump size the moment it was clicked.
const displayClasses =
  'cursor-text rounded border border-transparent px-2 py-1 text-left text-sm hover:border-edge-strong/40 hover:bg-surface-hover'

const inputClasses =
  'rounded border border-blue bg-canvas px-2 py-1 text-sm text-ink-strong'

type CommonProps = {
  label: string
  queryKey: readonly unknown[]
  disabled?: boolean
}

export function EditableText({
  value,
  onSave,
  label,
  queryKey,
  placeholder = 'Not set',
  monospace = false,
  secret = false,
  multiline = false,
  validate,
  disabled,
}: CommonProps & {
  value: string
  onSave: (next: string) => Promise<unknown>
  placeholder?: string
  monospace?: boolean
  // secret masks the value until revealed. The config API returns API keys in
  // cleartext, so anything credential-shaped is masked by default rather than
  // sitting on screen for whoever walks past.
  secret?: boolean
  multiline?: boolean
  validate?: (next: string) => string | null
}) {
  const { state, error, displayValue, save, dismissError } =
    useSaveState<string>({ serverValue: value, queryKey })

  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value)
  const [revealed, setRevealed] = useState(false)
  const [validationError, setValidationError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement | HTMLTextAreaElement>(null)

  useEffect(() => {
    if (editing) {
      inputRef.current?.focus()
      inputRef.current?.select()
    }
  }, [editing])

  function begin() {
    if (disabled) return
    setDraft(displayValue)
    setValidationError(null)
    setEditing(true)
  }

  async function commit() {
    if (!editing) return
    const next = draft.trim()
    if (next === displayValue) {
      setEditing(false)
      return
    }

    const problem = validate?.(next) ?? null
    if (problem) {
      setValidationError(problem)
      return
    }

    setEditing(false)
    await save(next, () => onSave(next))
  }

  function cancel() {
    setEditing(false)
    setValidationError(null)
  }

  if (editing) {
    const shared = {
      ref: inputRef as never,
      value: draft,
      onChange: (
        event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
      ) => setDraft(event.target.value),
      onBlur: () => void commit(),
      'aria-label': label,
      className: `${inputClasses} w-full ${monospace ? 'font-mono' : ''}`,
    }

    return (
      <div className="flex flex-col gap-1">
        {multiline ? (
          <textarea
            {...shared}
            rows={3}
            onKeyDown={(event) => {
              if (event.key === 'Escape') cancel()
            }}
          />
        ) : (
          <input
            {...shared}
            type={secret && !revealed ? 'text' : 'text'}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                void commit()
              }
              if (event.key === 'Escape') cancel()
            }}
          />
        )}
        {validationError ? (
          <span className="text-xs text-red">{validationError}</span>
        ) : (
          <span className="text-xs text-ink-muted">
            Enter to save, Escape to cancel
          </span>
        )}
      </div>
    )
  }

  const isEmpty = displayValue === ''
  const shown =
    secret && !revealed && !isEmpty ? '•'.repeat(Math.min(displayValue.length, 32)) : displayValue

  return (
    <div className="flex items-center gap-2">
      <button
        type="button"
        onClick={begin}
        disabled={disabled}
        aria-label={`Edit ${label}`}
        className={`${displayClasses} w-full ${monospace ? 'font-mono' : ''} ${
          isEmpty ? 'text-ink-muted italic' : 'text-ink-strong'
        } ${disabled ? 'cursor-not-allowed opacity-60' : ''} ${
          multiline ? 'whitespace-pre-wrap' : 'truncate'
        }`}
      >
        {isEmpty ? placeholder : shown}
      </button>
      {secret && !isEmpty ? (
        <button
          type="button"
          onClick={() => setRevealed((current) => !current)}
          className="shrink-0 text-xs text-ink-muted hover:text-ink-strong"
        >
          {revealed ? 'hide' : 'show'}
        </button>
      ) : null}
      <SaveIndicator state={state} error={error} onDismissError={dismissError} />
    </div>
  )
}

export function EditableNumber({
  value,
  onSave,
  label,
  queryKey,
  min,
  disabled,
  validate,
}: CommonProps & {
  value: number
  onSave: (next: number) => Promise<unknown>
  min?: number
  validate?: (next: number) => string | null
}) {
  const { state, error, displayValue, save, dismissError } =
    useSaveState<number>({ serverValue: value, queryKey })

  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(String(value))
  const [validationError, setValidationError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (editing) {
      inputRef.current?.focus()
      inputRef.current?.select()
    }
  }, [editing])

  async function commit() {
    if (!editing) return
    const parsed = Number(draft)

    if (!Number.isFinite(parsed) || !Number.isInteger(parsed)) {
      setValidationError('Must be a whole number')
      return
    }
    if (min !== undefined && parsed < min) {
      setValidationError(`Must be ${min} or more`)
      return
    }
    const problem = validate?.(parsed) ?? null
    if (problem) {
      setValidationError(problem)
      return
    }

    setEditing(false)
    if (parsed === displayValue) return
    await save(parsed, () => onSave(parsed))
  }

  if (editing) {
    return (
      <div className="flex flex-col gap-1">
        <input
          ref={inputRef}
          type="number"
          value={draft}
          aria-label={label}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={() => void commit()}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              void commit()
            }
            if (event.key === 'Escape') setEditing(false)
          }}
          className={`${inputClasses} w-32`}
        />
        {validationError ? (
          <span className="text-xs text-red">{validationError}</span>
        ) : null}
      </div>
    )
  }

  return (
    <div className="flex items-center gap-2">
      <button
        type="button"
        disabled={disabled}
        aria-label={`Edit ${label}`}
        onClick={() => {
          setDraft(String(displayValue))
          setValidationError(null)
          setEditing(true)
        }}
        className={`${displayClasses} w-32 text-ink-strong tabular-nums`}
      >
        {displayValue}
      </button>
      <SaveIndicator state={state} error={error} onDismissError={dismissError} />
    </div>
  )
}

export function EditableSelect({
  value,
  options,
  onSave,
  label,
  queryKey,
  disabled,
}: CommonProps & {
  value: string
  options: readonly string[]
  onSave: (next: string) => Promise<unknown>
}) {
  const { state, error, displayValue, save, dismissError } =
    useSaveState<string>({ serverValue: value, queryKey })

  return (
    <div className="flex items-center gap-2">
      <select
        value={displayValue}
        aria-label={label}
        disabled={disabled}
        onChange={(event) => {
          const next = event.target.value
          if (next !== displayValue) {
            void save(next, () => onSave(next))
          }
        }}
        className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm text-ink-strong hover:border-edge-strong"
      >
        {/* A stored value outside the vocabulary still has to be selectable,
            or the select would silently rewrite it on the next save. */}
        {!options.includes(displayValue) ? (
          <option value={displayValue}>{displayValue || '—'}</option>
        ) : null}
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
      <SaveIndicator state={state} error={error} onDismissError={dismissError} />
    </div>
  )
}
