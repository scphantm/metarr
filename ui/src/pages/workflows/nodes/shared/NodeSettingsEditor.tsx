import { useState } from 'react'
import { createPortal } from 'react-dom'

import { Button } from '../../../../components/Card'
import type { Setting } from '../../catalogTypes'

/*
 * The edit form for one node's settings, opened from that node's Edit
 * button. Rendered through a portal to document.body — the node card lives
 * inside React Flow's zoomed/panned canvas transform, and a modal positioned
 * relative to that would be scaled and clipped along with it.
 *
 * Generalized from the old NodeParameterEditor: fields now come from the
 * catalog's Setting[] (name/type/default/ui/description) rather than the
 * pre-redesign ParameterDefinition[], and widgets are chosen from
 * setting.ui.widget rather than only the value's type.
 *
 * Also the one place per-instance color overrides are set — every editable
 * node gets this modal regardless of whether it has catalog settings (see
 * NodeShell's canEdit), since color is a property of the individual node,
 * not the node type (nodes/shared/nodeVisual.ts still supplies the default
 * when no override is set). Shape color and border color are independent
 * parameters with their own pickers — overriding one never touches the
 * other.
 */

// The app's 8 real Solarized accents — never a raw hex, matching every
// other color in this codebase. Excludes 'base1', which is a neutral
// fallback used for data handles, not a semantic accent a user would pick.
const COLOR_TOKENS = ['red', 'orange', 'yellow', 'green', 'cyan', 'blue', 'violet', 'magenta'] as const

function ColorPicker({
  label,
  value,
  onChange,
}: {
  label: string
  value: string | undefined
  onChange: (next: string | undefined) => void
}) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs text-ink-muted">{label}</span>
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={() => onChange(undefined)}
          aria-label={`Use the default ${label.toLowerCase()} for this node type`}
          title="Default"
          className={`flex h-6 w-6 items-center justify-center rounded-full border-2 text-[10px] text-ink-muted ${
            value === undefined ? 'border-blue' : 'border-edge-strong/40'
          }`}
        >
          ✕
        </button>
        {COLOR_TOKENS.map((token) => (
          <button
            key={token}
            type="button"
            onClick={() => onChange(token)}
            aria-label={token}
            title={token}
            className={`h-6 w-6 rounded-full border-2 ${value === token ? 'border-blue' : 'border-transparent'}`}
            style={{ backgroundColor: `var(--color-${token})` }}
          />
        ))}
      </div>
    </div>
  )
}

export function NodeSettingsEditor({
  nodeName,
  typeKey,
  description,
  settings,
  values,
  shapeColor,
  borderColor,
  onSave,
  onCancel,
}: {
  nodeName: string
  typeKey: string
  description?: string
  settings: Setting[]
  values: Record<string, unknown>
  shapeColor?: string
  borderColor?: string
  onSave: (next: { settings: Record<string, unknown>; shapeColor?: string; borderColor?: string }) => void
  onCancel: () => void
}) {
  const [draft, setDraft] = useState<Record<string, unknown>>(values)
  const [shapeColorDraft, setShapeColorDraft] = useState<string | undefined>(shapeColor)
  const [borderColorDraft, setBorderColorDraft] = useState<string | undefined>(borderColor)

  function setValue(name: string, value: unknown) {
    setDraft((current) => ({ ...current, [name]: value }))
  }

  function valueFor(setting: Setting) {
    const current = draft[setting.name]
    return current === undefined ? setting.default : current
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onCancel}>
      <div
        className="w-full max-w-md rounded-lg border border-edge bg-surface p-5 shadow-lg"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 className="text-sm font-semibold text-ink-strong">{nodeName}</h2>
        <div className="mt-0.5 font-mono text-xs text-ink-muted">{typeKey}</div>
        {description ? <p className="mt-1.5 text-[11px] text-ink-muted">{description}</p> : null}

        <div className="mt-4 flex flex-col gap-3 border-t border-edge/60 pt-4">
          <ColorPicker label="Shape color" value={shapeColorDraft} onChange={setShapeColorDraft} />
          <ColorPicker label="Border color" value={borderColorDraft} onChange={setBorderColorDraft} />
        </div>

        <div className="mt-4 flex flex-col gap-3 border-t border-edge/60 pt-4">
          {settings.length === 0 ? (
            <p className="text-xs text-ink-muted">This node has no configurable settings.</p>
          ) : null}
          {settings.map((setting) => {
            const fieldId = `setting-${setting.name}`
            const widget = setting.ui?.widget as string | undefined
            const options = Array.isArray(setting.ui?.options) ? (setting.ui?.options as string[]) : []
            const value = valueFor(setting)

            return (
              <div key={setting.name} className="flex flex-col gap-1">
                <label className="text-xs text-ink-muted" htmlFor={fieldId}>
                  {setting.label ?? setting.name}
                </label>

                {setting.type === 'bool' ? (
                  <input
                    id={fieldId}
                    type="checkbox"
                    checked={Boolean(value)}
                    onChange={(event) => setValue(setting.name, event.target.checked)}
                    className="h-4 w-4 self-start"
                  />
                ) : widget === 'dropdown' && options.length > 0 ? (
                  <select
                    id={fieldId}
                    value={value == null ? '' : String(value)}
                    onChange={(event) => setValue(setting.name, event.target.value)}
                    className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm text-ink-strong focus:border-blue"
                  >
                    {options.map((option) => (
                      <option key={option} value={option}>
                        {option}
                      </option>
                    ))}
                  </select>
                ) : widget === 'textarea' ? (
                  <textarea
                    id={fieldId}
                    value={value == null ? '' : String(value)}
                    onChange={(event) => setValue(setting.name, event.target.value)}
                    rows={3}
                    className="resize-none rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm text-ink-strong focus:border-blue"
                  />
                ) : (
                  <input
                    id={fieldId}
                    type={setting.type === 'number' || setting.type === 'number.int' ? 'number' : 'text'}
                    value={value == null ? '' : String(value)}
                    onChange={(event) =>
                      setValue(
                        setting.name,
                        setting.type === 'number' || setting.type === 'number.int'
                          ? Number(event.target.value)
                          : event.target.value,
                      )
                    }
                    className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm text-ink-strong focus:border-blue"
                  />
                )}

                {setting.description ? <p className="text-[11px] text-ink-muted">{setting.description}</p> : null}
              </div>
            )
          })}
        </div>

        <div className="mt-5 flex justify-end gap-2">
          <Button variant="default" onClick={onCancel}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={() => onSave({ settings: draft, shapeColor: shapeColorDraft, borderColor: borderColorDraft })}
          >
            Save
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
