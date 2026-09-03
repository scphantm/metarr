import type { Edge, Node, Viewport } from "@xyflow/react";

/*
 * Stashes the in-progress editor draft for one workflow while its old
 * versions are being browsed read-only, so switching back doesn't lose
 * unsaved work. Mirrors ThemeContext's flat metarr.-prefixed, try/catch
 * convention — the only localStorage precedent in this codebase — since no
 * shared wrapper exists to extend.
 */

const keyFor = (id: string) => `metarr.workflow-draft.${id}`;

export type StashedDraft = {
  name: string;
  description: string;
  tags: string[];
  nodes: Node[];
  edges: Edge[];
  viewport: Viewport;
};

export function stashDraft(id: string, draft: StashedDraft): void {
  try {
    localStorage.setItem(keyFor(id), JSON.stringify(draft));
  } catch {
    // Best effort — losing the stash just means "Back to editing" falls back
    // to the last-loaded version instead of the in-progress draft.
  }
}

export function readStashedDraft(id: string): StashedDraft | null {
  try {
    const raw = localStorage.getItem(keyFor(id));
    return raw ? (JSON.parse(raw) as StashedDraft) : null;
  } catch {
    return null;
  }
}

export function clearStashedDraft(id: string): void {
  try {
    localStorage.removeItem(keyFor(id));
  } catch {
    // Best effort.
  }
}
