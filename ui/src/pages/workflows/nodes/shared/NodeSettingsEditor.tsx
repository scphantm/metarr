import { useState } from 'react'
import { Checkbox, Input, InputNumber, Modal, Select, Typography } from 'antd'

import type { Setting } from '../../catalogTypes'
import './NodeSettingsEditor.css'

/*
 * The edit form for one node's settings, opened from that node's Edit
 * button. Rendered through antd's Modal, which portals to document.body —
 * the node card lives inside React Flow's zoomed/panned canvas transform,
 * and a modal positioned relative to that would be scaled and clipped along
 * with it.
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

// No antd component picks from a fixed palette of semantic tokens (its own
// ColorPicker is a full HSB dialog) — this stays a small bespoke swatch row.
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
    <div className="node-settings-color-picker">
      <span className="node-settings-color-picker-label">{label}</span>
      <div className="node-settings-color-swatches">
        <button
          type="button"
          onClick={() => onChange(undefined)}
          aria-label={`Use the default ${label.toLowerCase()} for this node type`}
          title="Default"
          className={`node-settings-color-swatch is-default ${value === undefined ? 'is-selected' : ''}`}
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
            className={`node-settings-color-swatch ${value === token ? 'is-selected' : ''}`}
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

  return (
    <Modal
      open
      title={
        <div>
          <div>{nodeName}</div>
          <Typography.Text type="secondary" className="node-settings-type-key">
            {typeKey}
          </Typography.Text>
        </div>
      }
      onCancel={onCancel}
      onOk={() => onSave({ settings: draft, shapeColor: shapeColorDraft, borderColor: borderColorDraft })}
      okText="Save"
    >
      {description ? (
        <Typography.Text type="secondary" className="node-settings-description">
          {description}
        </Typography.Text>
      ) : null}

      <div className="node-settings-section">
        <ColorPicker label="Shape color" value={shapeColorDraft} onChange={setShapeColorDraft} />
        <ColorPicker label="Border color" value={borderColorDraft} onChange={setBorderColorDraft} />
      </div>

      <div className="node-settings-section">
        {settings.length === 0 ? (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            This node has no configurable settings.
          </Typography.Text>
        ) : null}
        {settings.map((setting) => {
          const fieldId = `setting-${setting.name}`
          const widget = setting.ui?.widget as string | undefined
          const options = Array.isArray(setting.ui?.options) ? (setting.ui?.options as string[]) : []
          const value = valueFor(setting)

          return (
            <div key={setting.name} className="node-settings-field">
              <label className="node-settings-field-label" htmlFor={fieldId}>
                {setting.label ?? setting.name}
              </label>

              {setting.type === 'bool' ? (
                <Checkbox
                  id={fieldId}
                  checked={Boolean(value)}
                  onChange={(event) => setValue(setting.name, event.target.checked)}
                />
              ) : widget === 'dropdown' && options.length > 0 ? (
                <Select
                  id={fieldId}
                  value={value == null ? '' : String(value)}
                  onChange={(next) => setValue(setting.name, next)}
                  options={options.map((option) => ({ value: option, label: option }))}
                />
              ) : widget === 'textarea' ? (
                <Input.TextArea
                  id={fieldId}
                  value={value == null ? '' : String(value)}
                  onChange={(event) => setValue(setting.name, event.target.value)}
                  rows={3}
                />
              ) : (
                (() => {
                  // number.int was retired from the type lattice (design.md
                  // §4.1) — those settings now declare "any", so the
                  // declared type alone can no longer pick the number
                  // widget. Falling back to the resolved value's actual JS
                  // type keeps former-number.int settings (tile width,
                  // branch count, ...) rendering as a number input rather
                  // than regressing to plain text.
                  const isNumeric = setting.type === 'number' || typeof value === 'number'
                  return isNumeric ? (
                    <InputNumber
                      id={fieldId}
                      value={value == null ? undefined : Number(value)}
                      onChange={(next) => setValue(setting.name, next ?? 0)}
                      style={{ width: '100%' }}
                    />
                  ) : (
                    <Input
                      id={fieldId}
                      value={value == null ? '' : String(value)}
                      onChange={(event) => setValue(setting.name, event.target.value)}
                    />
                  )
                })()
              )}

              {setting.description ? (
                <Typography.Text type="secondary" className="node-settings-field-hint">
                  {setting.description}
                </Typography.Text>
              ) : null}
            </div>
          )
        })}
      </div>
    </Modal>
  )
}
