import { useState } from 'react'
import { Input, Space, Tag } from 'antd'

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
    <Space size={6} wrap align="center">
      {value.map((tag) => (
        <Tag
          key={tag}
          closable
          onClose={(event) => {
            event.preventDefault()
            onChange(value.filter((t) => t !== tag))
          }}
        >
          {tag}
        </Tag>
      ))}

      <Input
        value={draft}
        placeholder="Add a tag"
        aria-label="Add a tag"
        size="small"
        variant="borderless"
        style={{
          width: 112,
          borderStyle: 'dashed',
          borderWidth: 1,
          borderColor: 'var(--surface-edge-strong)',
        }}
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
      />
    </Space>
  )
}
