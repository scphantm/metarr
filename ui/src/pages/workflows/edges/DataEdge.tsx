import { useState, type CSSProperties } from "react";
import {
  BaseEdge,
  EdgeLabelRenderer,
  getBezierPath,
  Position,
  useReactFlow,
  type Edge,
  type EdgeProps,
} from "@xyflow/react";

import {
  iconClassForType,
  ITERATE_ICON_CLASS,
  RECURSIVE_ICON_CLASS,
  TYPE_UNSAFE_ICON_CLASS,
} from "../../../lib/typeIcons";
import { canConnect, parseHandleId } from "../connectionRules";
import type { CatalogNodeData } from "../editorNodeData";
import { useCatalogEntry, useTransforms } from "../useCatalogEntry";
import { useIconZoomVisibility } from "../useIconZoomVisibility";
import { EdgeSettingsEditor } from "./EdgeSettingsEditor";
import { TransformPicker } from "./TransformPicker";
import "./DataEdge.css";

// The unit direction a handle's own position "faces" — used to slide an
// endpoint icon along the edge, away from its node, toward the middle (see
// index.css's --data-edge-icon-offset). Both ends use this on their own
// position: a source's outward direction and a target's outward direction
// point into the path from opposite sides.
const OUTWARD_UNIT: Record<Position, { ux: number; uy: number }> = {
  [Position.Top]: { ux: 0, uy: -1 },
  [Position.Bottom]: { ux: 0, uy: 1 },
  [Position.Left]: { ux: -1, uy: 0 },
  [Position.Right]: { ux: 1, uy: 0 },
};

/*
 * The one shared data-edge component: thin, static, theme orange (the wire
 * marks it as a data edge at a glance, distinct from a cyan control edge —
 * NodeShell.tsx's socket dots match these same two colors now, so an edge
 * and the sockets it connects read as one visual family; a specific type is
 * conveyed by the icon mask instead, see lib/typeIcons.ts), dashed when a
 * transform is applied. The transform — if any — shows as a clickable chip
 * (design.md §4.4's "small chip on the edge reading e.g. parentDir");
 * clicking it reopens
 * TransformPicker to change or clear the conversion. All of this is real CSS
 * (index.css's .data-edge-* classes) — the component only ever passes
 * coordinates and a unit direction across as custom-property values, never
 * composes styling in JS.
 *
 * Both ends also show the type icon for that end's socket (lib/typeIcons.ts,
 * same lookup NodeShell.tsx's handles use — single source of truth), and the
 * source end additionally shows an iteration badge when the active
 * transform has implies_iteration set (design.md §4.3's ETL transforms,
 * e.g. eachFile) — a data edge, not a control-flow construct. The target end
 * shows a warning badge instead when the connection itself is TypeUnsafe
 * (design.md §4.3's narrowing rule) — generic, not scoped to any one type
 * family. These endpoint icons only show at maximum zoom (useIconZoomVisibility,
 * shared with NodeShell.tsx's handle icons) — the transform chip does not,
 * since it's a text label rather than a small icon.
 *
 * Double-clicking a path-typed edge (target socket type in the `path`
 * family) opens EdgeSettingsEditor, currently offering just "Recursive".
 * When set, a FaFolderTree badge joins the target-end icon group, titled
 * "Path destinations are recursive". Gated on the target end because
 * recursion describes what the receiving node does with the destination,
 * not the source — a transform (e.g. media.file.parentDir) can make the
 * two ends different type families.
 */

// diagnosticHighlight is set by WorkflowCanvas.tsx while the user hovers a
// diagnostic naming this edge — see DiagnosticsPanel.tsx — never persisted
// (graphAdapter.ts's fromRFEdge doesn't read it).
export type DataEdgeData = {
  transform?: string;
  diagnosticHighlight?: boolean;
  settings?: Record<string, unknown>;
};
export type DataEdgeType = Edge<DataEdgeData, "dataEdge">;

export function DataEdge({
  id,
  source,
  target,
  sourceHandleId,
  targetHandleId,
  sourceX,
  sourceY,
  sourcePosition,
  targetX,
  targetY,
  targetPosition,
  data,
  markerEnd,
  selected,
}: EdgeProps<DataEdgeType>) {
  const { getNode, updateEdgeData } = useReactFlow();
  const [pickerOpen, setPickerOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const transforms = useTransforms();
  const showSmallIcons = useIconZoomVisibility();

  const sourceNode = getNode(source);
  const targetNode = getNode(target);
  const sourceNodeType = useCatalogEntry(
    (sourceNode?.data as CatalogNodeData | undefined)?.catalogId,
    sourceNode?.type ?? "",
  );
  const targetNodeType = useCatalogEntry(
    (targetNode?.data as CatalogNodeData | undefined)?.catalogId,
    targetNode?.type ?? "",
  );

  const sourceSocketName = parseHandleId(sourceHandleId)?.name;
  const targetSocketName = parseHandleId(targetHandleId)?.name;
  const sourceSocket = sourceNodeType?.dataOut?.find(
    (socket) => socket.name === sourceSocketName,
  );
  const targetSocket = targetNodeType?.dataIn?.find(
    (socket) => socket.name === targetSocketName,
  );

  const [path, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });

  const connectionInfo =
    sourceSocket && targetSocket
      ? canConnect(sourceSocket.type, targetSocket.type, transforms)
      : undefined;
  const activeTransform = data?.transform
    ? transforms.find((candidate) => candidate.name === data.transform)
    : undefined;
  const sourceIconClass = sourceSocket
    ? iconClassForType(sourceSocket.type)
    : undefined;
  const targetIconClass = targetSocket
    ? iconClassForType(targetSocket.type)
    : undefined;
  const sourceUnit = OUTWARD_UNIT[sourcePosition];
  const targetUnit = OUTWARD_UNIT[targetPosition];
  const targetType = targetSocket?.type;
  const isPathEdge =
    targetType === "path" || (targetType?.startsWith("path.") ?? false);
  const isRecursive = Boolean(data?.settings?.recursive);

  return (
    <g
      className={data?.diagnosticHighlight ? "diagnostic-blink" : undefined}
      onDoubleClick={isPathEdge ? () => setSettingsOpen(true) : undefined}
    >
      <BaseEdge
        id={id}
        path={path}
        markerEnd={markerEnd}
        className={`data-edge-path${data?.transform ? " is-transformed" : ""}${selected ? " is-selected" : ""}`}
      />

      {data?.transform ? (
        <EdgeLabelRenderer>
          <div
            style={
              {
                "--edge-x": `${labelX}px`,
                "--edge-y": `${labelY}px`,
              } as CSSProperties
            }
            className="data-edge-transform-chip nodrag nopan pointer-events-auto"
          >
            <button
              type="button"
              onClick={() => setPickerOpen(true)}
              className="data-edge-chip-button"
            >
              {data.transform}
            </button>
          </div>
        </EdgeLabelRenderer>
      ) : null}

      {showSmallIcons && (sourceIconClass || targetIconClass) ? (
        <EdgeLabelRenderer>
          {sourceIconClass ? (
            <div
              style={
                {
                  "--edge-x": `${sourceX}px`,
                  "--edge-y": `${sourceY}px`,
                  "--edge-ux": sourceUnit.ux,
                  "--edge-uy": sourceUnit.uy,
                } as CSSProperties
              }
              className="data-edge-endpoint"
            >
              <span className={`${sourceIconClass} data-edge-icon`} />
              {activeTransform?.impliesIteration ? (
                <span className={`${ITERATE_ICON_CLASS} data-edge-icon`} />
              ) : null}
            </div>
          ) : null}
          {targetIconClass ? (
            <div
              style={
                {
                  "--edge-x": `${targetX}px`,
                  "--edge-y": `${targetY}px`,
                  "--edge-ux": targetUnit.ux,
                  "--edge-uy": targetUnit.uy,
                } as CSSProperties
              }
              className="data-edge-endpoint"
            >
              <span className={`${targetIconClass} data-edge-icon`} />
              {connectionInfo?.typeUnsafe ? (
                <span
                  className={`${TYPE_UNSAFE_ICON_CLASS} data-edge-icon pointer-events-auto`}
                  title="Not rigidly type safe — the connected value's actual type isn't guaranteed to match."
                />
              ) : null}
              {isRecursive ? (
                <span
                  className={`${RECURSIVE_ICON_CLASS} data-edge-icon pointer-events-auto`}
                  title="Path destinations are recursive"
                />
              ) : null}
            </div>
          ) : null}
        </EdgeLabelRenderer>
      ) : null}

      {pickerOpen && sourceSocket && targetSocket && connectionInfo ? (
        <TransformPicker
          fromType={sourceSocket.type}
          toType={targetSocket.type}
          candidates={connectionInfo.candidates}
          current={data?.transform}
          onPick={(name) => {
            void updateEdgeData(id, { transform: name });
            setPickerOpen(false);
          }}
          onClose={() => setPickerOpen(false)}
        />
      ) : null}

      {settingsOpen ? (
        <EdgeSettingsEditor
          recursive={isRecursive}
          onSave={(next) => {
            void updateEdgeData(id, {
              settings: { ...data?.settings, recursive: next.recursive },
            });
            setSettingsOpen(false);
          }}
          onCancel={() => setSettingsOpen(false)}
        />
      ) : null}
    </g>
  );
}
