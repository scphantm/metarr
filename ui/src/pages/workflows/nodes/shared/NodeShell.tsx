import { useState, type CSSProperties } from 'react'
import { Handle, Position, useReactFlow } from '@xyflow/react'

import { iconClassForControlPort, iconClassForType } from '../../../../lib/typeIcons'
import { controlHandleId } from '../../connectionRules'
import { useCatalogEntry } from '../../useCatalogEntry'
import { useIconZoomVisibility } from '../../useIconZoomVisibility'
import type { CatalogNodeData } from '../../catalogTypes'
import { EditIcon } from './EditIcon'
import { NodeSettingsEditor } from './NodeSettingsEditor'
import {
  accentTintClassForAccent,
  hoverBorderColorClassForAccent,
  nodeVisual,
  shapeColorClassForAccent,
  type Accent,
} from './nodeVisual'
import { errorHandleTitle, handleOffset, useNodeHandles, type ArrangedHandles } from './useNodeHandles'
import './NodeShell.css'

/*
 * The common chrome every catalog-driven node is built on: a transparent,
 * compact card sized to its shape (nodes/shared/nodeVisual.ts) — no header
 * or footer row. A neutral base02 border at rest, revealing a 40%-opacity
 * tint of the node's accent on hover; the shape fill, label, and
 * edit/destructive affordances overlay the shape itself rather than
 * sitting in their own rows; four
 * quadrant divs behind the shape carry a live notification signal
 * (data.quadrantColors), invisible unless something sets them. Both shape
 * and (hover) border color are independently overridable per instance
 * (data.shapeColor / data.borderColor). Ports/sockets/labels are always
 * resolved live from the fetched catalog by `typeKey` — never hardcoded per
 * node file — so the ~30 files under nodes/{core,fs,media,nfo,string}/ stay
 * thin wrappers rather than 30 copies of this logic. A node whose own body
 * needs to diverge (currently just Notes) skips this component and renders
 * itself directly instead.
 */

// Square-ish and cyan — matching ControlEdge.tsx's ordinary (non-error)
// control-edge color, so a control socket and the wire it connects to read
// as one visual family. The error control port is styled separately, below
// (red, matching ControlEdge.tsx's error-branch color).
const controlHandleClass = 'node-handle-control'

// Orange, matching DataEdge.tsx's data-edge-path color, same reasoning as
// controlHandleClass above. Flat rather than colored per type
// (typeColors.ts's old per-type accent scheme is retired): the icon mask
// (iconClassForType) is what conveys a specific type now, not the dot's
// fill color.
//
// node-handle-data's !important border/background overrides React Flow's
// own base CSS, which puts a 1px border (a light theme color, not ours) on
// every handle by default. That border sits outside the mask-image
// entirely — mask-image only clips the background fill, never the border —
// so it's invisible only where the icon's own opaque area happens to reach
// the box edge. An icon that doesn't fill its box on every side (most of
// them, once "contain"-fit into a square) leaves that stretch of default
// border exposed as a stray pale line. controlHandleClass doesn't have this
// problem: it sets an explicit border color matching its own fill, so the
// border and the mask gaps are the same color either way.
//
// Shape depends on whether an icon is showing. React Flow's own base CSS
// also sets every handle to a circle (border-radius: 100%) by default,
// which is fine for the plain color dot — but a circular clip crops a
// mask-image that fills its box, cutting away however much of the icon
// falls outside the inscribed circle. A wide glyph (a folder) loses much
// more area to that clip than a narrow one (a document), which is why
// swapping the folder icon alone didn't fix the visibility complaint this
// was diagnosing — the shape was clipping it regardless of which glyph was
// in the mask. Explicit here rather than left to inherit React Flow's
// default, since a library default silently changing shape out from under
// us would be easy to miss: square/rounded whenever the icon mask is
// visible, circular only when zoomed out and there's no icon to protect.
const dataHandleClass = 'node-handle-data'
const dataHandleIconShape = 'is-icon-shape'
const dataHandleDotShape = 'is-dot-shape'

// A data handle's flat color (dataHandleClass) and, when its type has one
// registered, an icon mask on top (iconClassForType) — see lib/typeIcons.ts.
// A type with no icon composes to just the color class, unchanged from
// before this existed. showIcon is useIconZoomVisibility() — the icon mask
// only shows at maximum zoom, leaving just the plain color dot otherwise;
// see that hook's comment for why.
function dataHandleAppearance(type: string, showIcon: boolean): string {
  const shape = showIcon ? dataHandleIconShape : dataHandleDotShape
  return `${dataHandleClass} ${shape} ${showIcon ? (iconClassForType(type) ?? '') : ''}`.trim()
}

// Same pattern as dataHandleAppearance, but for a control port — keyed by
// port name (iconClassForControlPort) rather than data Type. A port name
// with no icon composes to just controlHandleClass, unchanged.
function controlHandleAppearance(port: string, showIcon: boolean): string {
  return `${controlHandleClass} ${showIcon ? (iconClassForControlPort(port) ?? '') : ''}`.trim()
}

const SHAPE_BOX_SIZE: CSSProperties = { width: 80, height: 52 }

export function NodeShell({
  id,
  data,
  typeKey,
  handles: handlesOverride,
}: {
  id: string
  data: CatalogNodeData
  typeKey: string
  // Pre-arranged handles to render instead of the catalog's full set — used
  // by node types whose visible port count depends on their own settings
  // rather than being fixed by the catalog alone (Parallel, Join; see
  // nodes/shared/branchPorts.ts).
  handles?: ArrangedHandles
}) {
  const { updateNodeData } = useReactFlow()
  const [editing, setEditing] = useState(false)
  const nodeType = useCatalogEntry(data.catalogId, typeKey)
  const catalogHandles = useNodeHandles(nodeType)
  const handles = handlesOverride ?? catalogHandles
  const showSmallIcons = useIconZoomVisibility()

  if (!nodeType) {
    // The catalog hasn't loaded yet, or (should not happen if
    // nodes/registry.ts is generated from the same catalog) this type has
    // vanished from it since the page loaded.
    return <div className="node-shell-fallback">{typeKey}</div>
  }

  const label = data.label ?? nodeType.name
  const settings = nodeType.settings ?? []
  // Every non-readonly node is editable now, not just ones with catalog
  // settings — color is a per-instance property of the node, not the node
  // type, so even a settings-less node (e.g. core/collect) still needs a
  // way to reach NodeSettingsEditor's color picker.
  const canEdit = !data.readOnly
  // The node's declared visual identity — shape, shape-fill accent, border
  // accent — resolved from its catalog type alone (see nodeVisual.ts), one
  // explicit entry per type, nothing shared or derived. Shape color and
  // border color are independent parameters, each with its own optional
  // per-instance override (data.shapeColor / data.borderColor, set via
  // NodeSettingsEditor) — overriding one never touches the other or the
  // node's shape.
  const visual = nodeVisual(nodeType.type)
  const shapeAccent = (data.shapeColor as Accent | undefined) ?? visual.shapeAccent
  const shapeColorClass = shapeColorClassForAccent(shapeAccent)
  // The border sits at a neutral base02 at rest and only reveals a
  // 40%-opacity tint of an accent on hover — data.borderColor, when set, is
  // what that hover accent is (still an independent per-instance override),
  // defaulting to the shape's own accent so hovering an unstyled node
  // reveals its shape color.
  const hoverBorderClass = hoverBorderColorClassForAccent((data.borderColor as Accent | undefined) ?? shapeAccent)
  const quadrantColors = data.quadrantColors ?? []

  return (
    <div className={`node-shell ${hoverBorderClass}`}>
      {handles.top.map((handle, index) => (
        <Handle
          key={handle.id}
          id={handle.id}
          type="target"
          position={Position.Top}
          style={{ left: handleOffset(index, handles.top.length) }}
          className={
            handle.kind === 'control'
              ? controlHandleAppearance(handle.label, showSmallIcons)
              : dataHandleAppearance(handle.type ?? 'any', showSmallIcons)
          }
          title={handle.title}
        />
      ))}
      {handles.bottom.map((handle, index) => (
        <Handle
          key={handle.id}
          id={handle.id}
          type="source"
          position={Position.Bottom}
          style={{ left: handleOffset(index, handles.bottom.length) }}
          className={
            handle.kind === 'control'
              ? controlHandleAppearance(handle.label, showSmallIcons)
              : dataHandleAppearance(handle.type ?? 'any', showSmallIcons)
          }
          title={handle.title}
        />
      ))}
      {handles.hasError ? (
        <Handle
          id={controlHandleId('error')}
          type="source"
          position={Position.Right}
          className={`node-handle-error ${showSmallIcons ? (iconClassForControlPort('error') ?? '') : ''}`.trim()}
          title={errorHandleTitle}
        />
      ) : null}

      <div
        className={`box node-shell-shape-box ${shapeColorClass}`}
        style={SHAPE_BOX_SIZE}
        title={nodeType.description ? `${label} — ${nodeType.description}` : label}
      >
        {/*
         * Four quadrants behind the shape — invisible unless
         * data.quadrantColors assigns one, a live notification signal (see
         * CatalogNodeData.quadrantColors), not authored node styling.
         * pointer-events-none so they never intercept clicks/hover meant
         * for the shape, edit button, or the box's own title tooltip.
         */}
        <div
          className={`node-shell-quadrant top-left ${quadrantColors[0] ? accentTintClassForAccent(quadrantColors[0] as Accent, 40) : ''}`}
        />
        <div
          className={`node-shell-quadrant top-right ${quadrantColors[1] ? accentTintClassForAccent(quadrantColors[1] as Accent, 40) : ''}`}
        />
        <div
          className={`node-shell-quadrant bottom-left ${quadrantColors[2] ? accentTintClassForAccent(quadrantColors[2] as Accent, 40) : ''}`}
        />
        <div
          className={`node-shell-quadrant bottom-right ${quadrantColors[3] ? accentTintClassForAccent(quadrantColors[3] as Accent, 40) : ''}`}
        />
        {visual.shapeIsIcon ? (
          <div className={`shape-icon ${visual.shapeClassName} ${visual.shapeExtraClassName ?? ''}`} />
        ) : (
          <div className={`shape ${visual.shapeClassName} ${visual.shapeExtraClassName ?? ''}`} />
        )}
        <div className="node-shell-label-wrap">
          <span className="node-shell-label">{label}</span>
        </div>
        {canEdit ? (
          <button
            type="button"
            onClick={() => setEditing(true)}
            aria-label={`Edit ${label} settings`}
            className="node-shell-edit-button"
          >
            <EditIcon />
          </button>
        ) : null}
        {nodeType.exec.effects === 'destructive' ? (
          <span
            title="Destructive: deletes or overwrites existing content"
            className="node-shell-destructive-dot"
          />
        ) : null}
      </div>

      {editing ? (
        <NodeSettingsEditor
          nodeName={label}
          typeKey={nodeType.type}
          description={nodeType.description}
          settings={settings}
          values={data.settings}
          shapeColor={data.shapeColor}
          borderColor={data.borderColor}
          onSave={(next) => {
            updateNodeData(id, next)
            setEditing(false)
          }}
          onCancel={() => setEditing(false)}
        />
      ) : null}
    </div>
  )
}
