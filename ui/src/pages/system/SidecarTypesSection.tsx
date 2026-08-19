import { useMemo, useState } from 'react'

import {
  queryKeys,
  useDeleteSidecarType,
  useReorderSidecarTypes,
  useResetSidecarTypes,
  useUpsertSidecarType,
} from '../../api/queries'
import {
  sidecarCategories,
  type SidecarTypeDefinition,
} from '../../api/types'
import { Button, Card, Row } from '../../components/Card'
import { EditableSelect, EditableText } from '../../components/Editable'
import { EditableList } from '../../components/EditableList'
import { SaveIndicator } from '../../components/SaveState'

/*
 * The sidecar classification table: the rules deciding what a non-media file
 * found next to a movie or episode is.
 *
 * Three rules come from the Go side and shape this editor:
 *
 *  - Order is the evaluation sequence, and the scanner takes the first enabled
 *    entry that accepts a file. It is changed only through the dedicated
 *    ordering endpoint, which covers the whole table at once, because
 *    uniqueness is a property of the table rather than of a row. So ordering is
 *    a move-up/move-down interaction that submits every id at once, never a
 *    number typed into a row.
 *
 *  - Order zero means disabled: kept in the table, still editable, never
 *    evaluated. That is an enable toggle here, not a number.
 *
 *  - Two enabled entries sharing an order make the table ambiguous and the
 *    registry refuses it outright — which would drop the scanner back to its
 *    built-in defaults. Renumbering from scratch on every reorder is what keeps
 *    that from ever being sent.
 */
export function SidecarTypesSection({
  types,
}: {
  types: SidecarTypeDefinition[]
}) {
  const upsertMutation = useUpsertSidecarType()
  const remove = useDeleteSidecarType()
  const reorder = useReorderSidecarTypes()
  const reset = useResetSidecarTypes()

  const [adding, setAdding] = useState(false)
  const [expanded, setExpanded] = useState<string | null>(null)

  // The upsert endpoint rejects any non-zero order outright — order belongs to
  // the ordering transaction, and sending one here is treated as a mistake
  // worth surfacing rather than something to ignore. Editing an enabled entry
  // would otherwise always fail, since its stored order is non-zero. The server
  // keeps the entry's existing order when updating, so zero loses nothing.
  const upsert = {
    mutateAsync: (entry: SidecarTypeDefinition) =>
      upsertMutation.mutateAsync({ ...entry, order: 0 }),
  }

  // Enabled entries in evaluation order, then the disabled ones — which have no
  // position of their own, so they sort last rather than bunching at the front
  // where order zero would otherwise put them.
  const sorted = useMemo(() => {
    const enabled = types
      .filter((entry) => entry.order !== 0)
      .sort((a, b) => a.order - b.order)
    const disabled = types
      .filter((entry) => entry.order === 0)
      .sort((a, b) => a.type.localeCompare(b.type))
    return [...enabled, ...disabled]
  }, [types])

  const enabledCount = sorted.filter((entry) => entry.order !== 0).length

  // Renumbers the enabled entries 10, 20, 30… in their new positions and sends
  // the whole map. Gaps of ten leave room to hand-place an entry later without
  // a full renumber.
  function submitOrder(nextEnabled: SidecarTypeDefinition[]) {
    const orders: Record<string, number> = {}
    nextEnabled.forEach((entry, index) => {
      orders[entry.id] = (index + 1) * 10
    })
    types
      .filter((entry) => !nextEnabled.some((moved) => moved.id === entry.id))
      .forEach((entry) => {
        orders[entry.id] = 0
      })
    return reorder.mutateAsync(orders)
  }

  function move(id: string, direction: -1 | 1) {
    const enabled = sorted.filter((entry) => entry.order !== 0)
    const index = enabled.findIndex((entry) => entry.id === id)
    const target = index + direction
    if (index < 0 || target < 0 || target >= enabled.length) return

    const next = [...enabled]
    ;[next[index], next[target]] = [next[target], next[index]]
    void submitOrder(next)
  }

  function setEnabled(entry: SidecarTypeDefinition, enabled: boolean) {
    const currentlyEnabled = sorted.filter((item) => item.order !== 0)
    const next = enabled
      ? [...currentlyEnabled, entry]
      : currentlyEnabled.filter((item) => item.id !== entry.id)
    void submitOrder(next)
  }

  return (
    <Card
      title="Sidecar types"
      description="How the scanner classifies non-media files. Rules are evaluated top to bottom and the first one that accepts a file wins, so narrower rules belong above the catch-alls."
      actions={
        <>
          <Button variant="primary" onClick={() => setAdding(true)}>
            Add type
          </Button>
          <Button
            variant="danger"
            onClick={() => {
              if (
                window.confirm(
                  'Reset the sidecar table to the built-in defaults? Every custom type and every edit to a built-in is discarded.',
                )
              ) {
                void reset.mutateAsync()
              }
            }}
          >
            Reset to defaults
          </Button>
        </>
      }
    >
      <div className="mb-3 flex items-center gap-3 text-xs text-ink-muted">
        <span>
          {enabledCount} enabled, {sorted.length - enabledCount} disabled
        </span>
        {reorder.isPending ? <SaveIndicator state="saving" /> : null}
      </div>

      <div className="flex flex-col gap-1.5">
        {sorted.map((entry, index) => {
          const isEnabled = entry.order !== 0
          const enabledIndex = sorted
            .filter((item) => item.order !== 0)
            .findIndex((item) => item.id === entry.id)

          return (
            <div
              key={entry.id}
              className={`rounded border ${
                isEnabled ? 'border-edge' : 'border-edge/50 opacity-60'
              }`}
            >
              <div className="flex flex-wrap items-center gap-2 px-3 py-2">
                {/* Stacked with a gap and their own hit areas: butted together
                    the two triangles read as a single glyph, and the reorder
                    affordance disappears. */}
                <div className="flex shrink-0 flex-col gap-1">
                  <button
                    type="button"
                    aria-label={`Move ${entry.type} earlier`}
                    disabled={!isEnabled || enabledIndex === 0}
                    onClick={() => move(entry.id, -1)}
                    className="flex h-4 w-5 items-center justify-center rounded text-[9px] leading-none text-ink-muted hover:bg-surface-hover hover:text-ink-strong disabled:opacity-25 disabled:hover:bg-transparent"
                  >
                    ▲
                  </button>
                  <button
                    type="button"
                    aria-label={`Move ${entry.type} later`}
                    disabled={!isEnabled || enabledIndex === enabledCount - 1}
                    onClick={() => move(entry.id, 1)}
                    className="flex h-4 w-5 items-center justify-center rounded text-[9px] leading-none text-ink-muted hover:bg-surface-hover hover:text-ink-strong disabled:opacity-25 disabled:hover:bg-transparent"
                  >
                    ▼
                  </button>
                </div>

                <span className="w-8 shrink-0 text-right font-mono text-xs text-ink-muted tabular-nums">
                  {isEnabled ? entry.order : '—'}
                </span>

                <button
                  type="button"
                  onClick={() =>
                    setExpanded(expanded === entry.id ? null : entry.id)
                  }
                  className="min-w-32 flex-1 text-left text-sm text-ink-strong hover:text-blue"
                >
                  {entry.type}
                </button>

                <span className="shrink-0 rounded bg-surface-hover px-1.5 py-0.5 text-xs text-ink-muted">
                  {entry.category}
                </span>

                <span className="hidden shrink-0 font-mono text-xs text-ink-muted sm:inline">
                  {entry.patterns.length} pattern
                  {entry.patterns.length === 1 ? '' : 's'}
                </span>

                <label className="flex shrink-0 items-center gap-1.5 text-xs text-ink-muted">
                  <input
                    type="checkbox"
                    checked={isEnabled}
                    onChange={(event) =>
                      setEnabled(entry, event.target.checked)
                    }
                  />
                  Enabled
                </label>

                <Button
                  variant="ghost"
                  onClick={() =>
                    setExpanded(expanded === entry.id ? null : entry.id)
                  }
                >
                  {expanded === entry.id ? 'Close' : 'Edit'}
                </Button>
              </div>

              {expanded === entry.id ? (
                <SidecarTypeDetail
                  entry={entry}
                  position={index}
                  onSave={(next) => upsert.mutateAsync(next)}
                  onRemove={() => {
                    if (
                      window.confirm(
                        `Delete the sidecar type "${entry.type}"? Files it used to classify become "unknown" on the next scan.`,
                      )
                    ) {
                      void remove.mutateAsync(entry.id)
                      setExpanded(null)
                    }
                  }}
                />
              ) : null}
            </div>
          )
        })}
      </div>

      {adding ? (
        <NewSidecarType
          existingTypes={types.map((entry) => entry.type)}
          onCancel={() => setAdding(false)}
          onCreate={async (entry) => {
            await upsert.mutateAsync(entry)
            setAdding(false)
          }}
        />
      ) : null}
    </Card>
  )
}

function SidecarTypeDetail({
  entry,
  position,
  onSave,
  onRemove,
}: {
  entry: SidecarTypeDefinition
  position: number
  onSave: (next: SidecarTypeDefinition) => Promise<unknown>
  onRemove: () => void
}) {
  const key = queryKeys.sidecarTypes

  return (
    <div className="border-t border-edge bg-canvas/40 px-4 py-2">
      <Row label="Type name" hint="Written onto every file classified this way">
        <EditableText
          label="Type name"
          queryKey={key}
          value={entry.type}
          validate={(next) => (next ? null : 'A type name is required')}
          onSave={(type) => onSave({ ...entry, type })}
        />
      </Row>

      <Row label="Category">
        <EditableSelect
          label="Category"
          queryKey={key}
          value={entry.category}
          options={sidecarCategories}
          onSave={(category) => onSave({ ...entry, category })}
        />
      </Row>

      <Row
        label="Patterns"
        hint="Go regular expressions, matched against the file name without its extension"
      >
        <EditableList
          label="Patterns"
          queryKey={key}
          values={entry.patterns}
          placeholder="(?i)^poster$"
          monospace
          emptyWarning="At least one pattern is required; the server rejects an empty list"
          validate={(pattern) => {
            // JavaScript and Go regexes are not the same dialect, but the
            // overlap covers everything used here, so this catches a typo
            // before it becomes a 400 from the server.
            try {
              new RegExp(pattern.replace(/\(\?i\)/g, ''))
              return null
            } catch {
              return 'Not a valid regular expression'
            }
          }}
          onSave={(patterns) => onSave({ ...entry, patterns })}
        />
      </Row>

      <Row
        label="Extensions"
        hint="Lowercase and dot-prefixed. An empty list accepts any extension."
      >
        <EditableList
          label="Extensions"
          queryKey={key}
          values={entry.extensions}
          placeholder=".jpg"
          monospace
          normalize={(extension) => {
            const lowered = extension.toLowerCase()
            return lowered.startsWith('.') ? lowered : `.${lowered}`
          }}
          onSave={(extensions) => onSave({ ...entry, extensions })}
        />
      </Row>

      <div className="flex items-center justify-between gap-3 py-2">
        <span className="font-mono text-xs text-ink-muted">
          id {entry.id} · position {position + 1}
        </span>
        <Button variant="danger" onClick={onRemove}>
          Delete type
        </Button>
      </div>
    </div>
  )
}

function NewSidecarType({
  existingTypes,
  onCreate,
  onCancel,
}: {
  existingTypes: string[]
  onCreate: (entry: SidecarTypeDefinition) => Promise<void>
  onCancel: () => void
}) {
  const [type, setType] = useState('')
  const [category, setCategory] = useState<string>(sidecarCategories[0])
  const [pattern, setPattern] = useState('')
  const [error, setError] = useState<string | null>(null)

  async function submit() {
    if (!type.trim()) {
      setError('A type name is required')
      return
    }
    if (existingTypes.includes(type.trim())) {
      setError('That type name is already in the table')
      return
    }
    if (!pattern.trim()) {
      setError('At least one pattern is required')
      return
    }
    setError(null)

    await onCreate({
      // An empty id is what asks the server to create. It mints the id itself
      // and rejects one chosen by the caller with a 404, so this must not be a
      // generated UUID.
      id: '',
      type: type.trim(),
      category,
      // New entries are created disabled by the server regardless; sending a
      // non-zero order here is rejected outright.
      order: 0,
      patterns: [pattern.trim()],
      extensions: [],
    })
  }

  return (
    <div className="mt-3 flex flex-col gap-2 rounded border border-dashed border-blue/60 px-4 py-3">
      <input
        autoFocus
        value={type}
        placeholder="Type name, e.g. storyboard"
        onChange={(event) => setType(event.target.value)}
        className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm text-ink-strong focus:border-blue"
      />
      <select
        value={category}
        onChange={(event) => setCategory(event.target.value)}
        className="w-56 rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm text-ink-strong"
      >
        {sidecarCategories.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
      <input
        value={pattern}
        placeholder="(?i)^storyboard$"
        onChange={(event) => setPattern(event.target.value)}
        className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 font-mono text-sm text-ink-strong focus:border-blue"
      />

      {error ? <span className="text-xs text-red">{error}</span> : null}
      <p className="text-xs text-ink-muted">
        Added disabled. Enable it once you have placed it in the evaluation
        order.
      </p>

      <div className="flex gap-2">
        <Button variant="primary" onClick={() => void submit()}>
          Add
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  )
}
