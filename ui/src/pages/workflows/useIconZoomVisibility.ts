import { useStore } from '@xyflow/react'

// The small per-connection type icons (data handles — nodes/shared/
// NodeShell.tsx; data edge endpoints — edges/DataEdge.tsx) only show when
// the canvas is at its maximum zoom — fully zoomed in, not just "not
// zoomed out much." maxZoom is read from the store rather than hardcoded
// so this stays correct if WorkflowCanvas.tsx ever sets its own maxZoom
// prop (today it relies on React Flow's default, 2). The epsilon absorbs
// floating-point drift from pinch/scroll gestures landing just under the
// cap rather than exactly on it.
const ZOOM_EPSILON = 1e-6

// A selector returning a primitive (not the whole viewport object, unlike
// useViewport()) so this only re-renders the caller when the boundary is
// actually crossed, not on every pan/zoom frame.
export function useIconZoomVisibility(): boolean {
  return useStore((state) => state.transform[2] >= state.maxZoom - ZOOM_EPSILON)
}
