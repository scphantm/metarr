import { createContext, use, useState, type ReactNode } from "react";

/*
 * Shares which catalog entry is currently being dragged from the palette
 * between the palette and the canvas, using plain native HTML5 drag-and-drop
 * — matching React Flow's own documented drag-and-drop example (no neodrag:
 * canvas node dragging and edge connection are React Flow's own built-in
 * systems regardless, and neodrag isn't documented for either).
 *
 * Only the entry's `id` is carried, not a whole catalog entry — the canvas
 * looks the rest up from the live fetched catalog on drop, which is the
 * single source of truth rather than whatever was serialized into
 * dataTransfer at drag-start. `id` alone is enough because it's the
 * catalog's unique key; `type` no longer is, since several entries may
 * share one (variations of a plugin, e.g. two core/start entries with
 * different dataOut shapes).
 */

export type DraggedNodeType = { id: string };

type DnDContextValue = {
  draggedTemplate: DraggedNodeType | null;
  setDraggedTemplate: (template: DraggedNodeType | null) => void;
};

const DnDContext = createContext<DnDContextValue | null>(null);

export function DnDProvider({ children }: { children: ReactNode }) {
  const [draggedTemplate, setDraggedTemplate] =
    useState<DraggedNodeType | null>(null);
  return (
    <DnDContext value={{ draggedTemplate, setDraggedTemplate }}>
      {children}
    </DnDContext>
  );
}

export function useDnD() {
  const context = use(DnDContext);
  if (!context) {
    throw new Error("useDnD must be used within a DnDProvider");
  }
  return context;
}
