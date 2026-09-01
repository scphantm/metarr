import { useState } from 'react'
import { Flex, Input, Space, Tag, Typography } from 'antd'

import { SaveIndicator } from './SaveState'
import { sameStringList, useSaveState } from './useSaveState'

/*
 * A string array edited as antd's documented "editable tags" pattern:
 * sidecar patterns, file extensions. Each tag can be edited in place or
 * removed (closable), and one dashed input at the end adds to the list.
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

  const monospaceClass = monospace ? 'editable-field-mono' : ''

  return (
    <Space direction="vertical" size={8} style={{ width: '100%' }}>
      <Flex wrap="wrap" gap={6} align="center">
        {displayValue.map((entry, index) =>
          editingIndex === index ? (
            <Input
              key={`${entry}-${index}`}
              autoFocus
              size="small"
              className={monospaceClass}
              style={{ width: 120 }}
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
            />
          ) : (
            <Tag
              key={`${entry}-${index}`}
              className={monospaceClass}
              closable
              onClose={(event) => {
                event.preventDefault()
                void commitList(displayValue.filter((_, i) => i !== index))
              }}
              onClick={() => {
                setEditingDraft(entry)
                setEditingIndex(index)
              }}
              style={{ cursor: 'pointer' }}
            >
              {entry}
            </Tag>
          ),
        )}

        <Input
          size="small"
          variant="borderless"
          className={monospaceClass}
          style={{
            width: 140,
            borderStyle: 'dashed',
            borderWidth: 1,
            borderColor: 'var(--surface-edge-strong)',
          }}
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
        />
      </Flex>

      <Space size={12} align="center">
        <SaveIndicator
          state={state}
          error={error}
          onDismissError={dismissError}
        />
        {entryError ? (
          <Typography.Text type="danger" style={{ fontSize: 12 }}>
            {entryError}
          </Typography.Text>
        ) : null}
        {displayValue.length === 0 && emptyWarning ? (
          <Typography.Text
            style={{ fontSize: 12, color: 'var(--color-orange)' }}
          >
            {emptyWarning}
          </Typography.Text>
        ) : null}
      </Space>
    </Space>
  )
}
