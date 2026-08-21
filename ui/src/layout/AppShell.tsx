import { useEffect, useState, type ReactNode } from 'react'

import { NavColumn } from './NavColumn'
import { Sidebar } from './Sidebar'

/*
 * The three-column shell: navigation, content, sidebar.
 *
 * Each column scrolls independently — the page itself never does — so the
 * navigation stays put while a long configuration page moves. Below lg the
 * outer columns fold away: the nav becomes a drawer, and the sidebar moves
 * under the content rather than being lost, since it carries the sign-out and
 * theme controls.
 */

// Persisted across sessions — pinning the nav column open is a standing
// preference, not something to re-choose every time the app loads.
const NAV_PINNED_KEY = 'metarr.nav.pinned'

export function AppShell({
  children,
  sidebar,
}: {
  children: ReactNode
  sidebar?: ReactNode
}) {
  const [navOpen, setNavOpen] = useState(false)
  const [panelOpen, setPanelOpen] = useState(false)
  // Hidden by default: collapsed to a thin hover strip so content gets the
  // width back until the nav is actually needed. Hovering the strip reveals
  // it as a floating overlay (doesn't reflow the grid); pinning switches it
  // to a normal, permanently-open column instead — see `navExpanded` below
  // and its two render branches.
  const [navPinned, setNavPinned] = useState(() => localStorage.getItem(NAV_PINNED_KEY) === 'true')
  const [navHovering, setNavHovering] = useState(false)

  useEffect(() => {
    localStorage.setItem(NAV_PINNED_KEY, String(navPinned))
  }, [navPinned])

  const navExpanded = navPinned || navHovering

  return (
    <div className="h-full bg-canvas text-ink">
      {/* Mobile bar: the only place the drawer can be opened from. */}
      <div className="flex items-center gap-3 border-b border-edge bg-surface px-4 py-2.5 lg:hidden">
        <button
          type="button"
          onClick={() => setNavOpen(true)}
          aria-label="Open navigation"
          className="rounded border border-edge-strong/40 px-2 py-1 text-sm text-ink-strong"
        >
          ☰
        </button>
        <span className="font-semibold text-ink-strong">Metarr</span>
      </div>

      {navOpen ? (
        <div className="fixed inset-0 z-40 lg:hidden">
          <button
            type="button"
            aria-label="Close navigation"
            onClick={() => setNavOpen(false)}
            className="absolute inset-0 bg-black/50"
          />
          <div className="absolute inset-y-0 left-0 w-64 overflow-y-auto border-r border-edge bg-surface">
            <NavColumn onNavigate={() => setNavOpen(false)} />
          </div>
        </div>
      ) : null}

      <div
        className="lg:grid lg:h-full"
        style={{ gridTemplateColumns: `${navPinned ? '16rem' : '0.75rem'} minmax(0,1fr) 0.75rem` }}
      >
        <div
          className="relative hidden border-r border-edge bg-surface lg:block"
          onMouseEnter={() => setNavHovering(true)}
          onMouseLeave={() => setNavHovering(false)}
        >
          {!navPinned ? (
            <div className="flex h-full w-3 items-center justify-center">
              <span className="h-8 w-0.5 rounded-full bg-edge-strong/40" />
            </div>
          ) : null}

          {navExpanded ? (
            <div
              className={
                navPinned
                  ? 'flex h-full w-64 flex-col overflow-y-auto'
                  : 'absolute inset-y-0 left-0 z-20 flex w-64 flex-col overflow-y-auto border-r border-edge bg-surface shadow-xl'
              }
            >
              <NavColumn pinned={navPinned} onTogglePin={() => setNavPinned((current) => !current)} />
            </div>
          ) : null}
        </div>

        <main className="overflow-y-auto">{children}</main>

        {/* Below lg this renders full-width and always visible, stacked under
            main, same as before. At lg and above it collapses to a thin strip
            that reclaims width for main, and pops the full panel out as an
            overlay on hover rather than pushing the layout. */}
        <div
          className="relative border-t border-edge bg-surface lg:h-full lg:border-t-0 lg:border-l"
          onMouseEnter={() => setPanelOpen(true)}
          onMouseLeave={() => setPanelOpen(false)}
        >
          <div
            className={`lg:absolute lg:top-0 lg:right-0 lg:z-10 lg:h-full lg:w-80 lg:border-l lg:border-edge lg:bg-surface lg:shadow-lg lg:transition-transform lg:duration-150 lg:ease-out ${
              panelOpen ? 'lg:translate-x-0' : 'lg:translate-x-[calc(100%-0.75rem)]'
            }`}
          >
            <Sidebar>{sidebar}</Sidebar>
          </div>
        </div>
      </div>
    </div>
  )
}

// PageHeader is the top of the centre column, kept here so every page presents
// its title the same way.
export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string
  description?: string
  actions?: ReactNode
}) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-4 border-b border-edge px-6 py-5">
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-ink-strong">
          {title}
        </h1>
        {description ? (
          <p className="mt-1 max-w-2xl text-sm text-ink-muted">{description}</p>
        ) : null}
      </div>
      {actions ? <div className="flex gap-2">{actions}</div> : null}
    </header>
  )
}
