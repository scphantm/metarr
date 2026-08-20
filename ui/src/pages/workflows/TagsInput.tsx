import { useState } from 'react'

/*
 * A plain controlled multi-value input, visually modeled on
 * components/EditableList.tsx's chip markup but with none of that
 * component's autosave/useSaveState machinery — this form has its own
 * explicit Save button, not autosave-on-blur.
 */

export function TagsInput({
  value,
  onChange,
}: {
  value: string[]
  onChange: (next: string[]) => void
}) {
  const [draft, setDraft] = useState('')

  function add() {
    const tag = draft.trim()
    setDraft('')
    if (tag && !value.includes(tag)) {
      onChange([...value, tag])
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {value.map((tag) => (
        <span
          key={tag}
          className="group inline-flex items-center gap-1 rounded border border-edge-strong/40 bg-surface-hover px-2 py-0.5 text-xs"
        >
          <span className="text-ink-strong">{tag}</span>
          <button
            type="button"
            aria-label={`Remove ${tag}`}
            onClick={() => onChange(value.filter((t) => t !== tag))}
            className="text-ink-muted hover:text-red"
          >
            ×
          </button>
        </span>
      ))}

      <input
        value={draft}
        placeholder="Add a tag"
        aria-label="Add a tag"
        onChange={(event) => setDraft(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.preventDefault()
            add()
          }
        }}
        onBlur={() => {
          if (draft.trim()) add()
        }}
        className="min-w-28 flex-1 rounded border border-dashed border-edge-strong/40 bg-transparent px-2 py-0.5 text-xs text-ink-strong placeholder:text-ink-muted focus:border-blue focus:border-solid"
      />
    </div>
  )
}
