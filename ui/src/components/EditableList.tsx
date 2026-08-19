import { useState } from 'react'

import { SaveIndicator } from './SaveState'
import { sameStringList, useSaveState } from './useSaveState'

/*
 * A string array edited as chips: sidecar patterns, file extensions. Each chip
 * can be edited in place or removed, and one input at the end adds to the list.
 *
 * The whole list saves as a unit, because that is how the API takes it — a
 * sidecar type is upserted whole, never field by field.
 */

export function EditableList({
  values,
  onSave,
  label,
  queryKey,
  placeholder,
  monospace = false,
  normalize,
  validate,
  emptyWarning,
}: {
  values: string[]
  onSave: (next: string[]) => Promise<unknown>
  label: string
  queryKey: readonly unknown[]
  placeholder: string
  monospace?: boolean
  // normalize runs before an entry is stored — extensions get lowercased and
  // dot-prefixed the same way the Go side does it.
  normalize?: (entry: string) => string
  validate?: (entry: string) => string | null
  // Shown when the list is empty and the server would reject it that way.
  emptyWarning?: string
}) {
  const { state, error, displayValue, save, dismissError } = useSaveState<
    string[]
  >({ serverValue: values, queryKey, isEqual: sameStringList })

  const [draft, setDraft] = useState('')
  const [editingIndex, setEditingIndex] = useState<number | null>(null)
  const [editingDraft, setEditingDraft] = useState('')
  const [entryError, setEntryError] = useState<string | null>(null)

  async function commitList(next: string[]) {
    await save(next, () => onSave(next))
  }

  function prepare(entry: string): string | null {
    const trimmed = entry.trim()
    if (!trimmed) return null
    const normalized = normalize ? normalize(trimmed) : trimmed
    const problem = validate?.(normalized) ?? null
    if (problem) {
      setEntryError(problem)
      return null
    }
    setEntryError(null)
    return normalized
  }

  async function add() {
    const entry = prepare(draft)
    if (!entry) return
    if (displayValue.includes(entry)) {
      setEntryError('Already in the list')
      return
    }
    setDraft('')
    await commitList([...displayValue, entry])
  }

  async function replaceAt(index: number) {
    const entry = prepare(editingDraft)
    setEditingIndex(null)
    if (!entry || entry === displayValue[index]) return
    const next = [...displayValue]
    next[index] = entry
    await commitList(next)
  }

  const monospaceClass = monospace ? 'font-mono' : ''

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-1.5">
        {displayValue.map((entry, index) =>
          editingIndex === index ? (
            <input
              key={`${entry}-${index}`}
              autoFocus
              value={editingDraft}
              aria-label={`Edit ${label} entry`}
              onChange={(event) => setEditingDraft(event.target.value)}
              onBlur={() => void replaceAt(index)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  void replaceAt(index)
                }
                if (event.key === 'Escape') setEditingIndex(null)
              }}
              className={`rounded border border-blue bg-canvas px-2 py-0.5 text-xs text-ink-strong ${monospaceClass}`}
            />
          ) : (
            <span
              key={`${entry}-${index}`}
              className="group inline-flex items-center gap-1 rounded border border-edge-strong/40 bg-surface-hover px-2 py-0.5 text-xs"
            >
              <button
                type="button"
                onClick={() => {
                  setEditingDraft(entry)
                  setEditingIndex(index)
                }}
                className={`text-ink-strong ${monospaceClass}`}
              >
                {entry}
              </button>
              <button
                type="button"
                aria-label={`Remove ${entry}`}
                onClick={() =>
                  void commitList(displayValue.filter((_, i) => i !== index))
                }
                className="text-ink-muted hover:text-red"
              >
                ×
              </button>
            </span>
          ),
        )}

        <input
          value={draft}
          placeholder={placeholder}
          aria-label={`Add to ${label}`}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              void add()
            }
          }}
          onBlur={() => {
            if (draft.trim()) void add()
          }}
          className={`min-w-40 flex-1 rounded border border-dashed border-edge-strong/40 bg-transparent px-2 py-0.5 text-xs text-ink-strong placeholder:text-ink-muted focus:border-blue focus:border-solid ${monospaceClass}`}
        />
      </div>

      <div className="flex items-center gap-3">
        <SaveIndicator
          state={state}
          error={error}
          onDismissError={dismissError}
        />
        {entryError ? (
          <span className="text-xs text-red">{entryError}</span>
        ) : null}
        {displayValue.length === 0 && emptyWarning ? (
          <span className="text-xs text-orange">{emptyWarning}</span>
        ) : null}
      </div>
    </div>
  )
}
