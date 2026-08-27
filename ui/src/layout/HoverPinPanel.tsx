import { useState, type CSSProperties, type ReactNode } from 'react'

import './HoverPinPanel.css'

/*
 * Shared hide/reveal-on-hover-with-pin behaviour for the left nav and right
 * panel (AppShell.tsx). Unpinned, the panel collapses to a thin strip that
 * gives its width back to the centre column; hovering (or tapping the strip,
 * for touch) reveals the full panel as a floating overlay that doesn't
 * reflow the layout. Pinning switches it to a normal, permanently-open
 * column instead.
 */
export function HoverPinPanel({
  side,
  pinned,
  width = '16rem',
  children,
}: {
  side: 'left' | 'right'
  pinned: boolean
  width?: string
  children: ReactNode
}) {
  const [hovering, setHovering] = useState(false)
  const expanded = pinned || hovering

  return (
    <div
      className={`hover-pin-panel hover-pin-panel-${side} ${pinned ? 'is-pinned' : ''}`}
      style={{ '--hover-pin-panel-width': width } as CSSProperties}
      onMouseEnter={() => setHovering(true)}
      onMouseLeave={() => setHovering(false)}
    >
      {!pinned ? (
        <button
          type="button"
          className="hover-pin-panel-strip"
          aria-label={`Show ${side} panel`}
          onClick={() => setHovering((current) => !current)}
        >
          <span className="hover-pin-panel-strip-handle" aria-hidden="true" />
        </button>
      ) : null}

      {expanded ? (
        <div className={`hover-pin-panel-content ${pinned ? 'is-pinned' : 'is-overlay'}`}>
          {children}
        </div>
      ) : null}
    </div>
  )
}
