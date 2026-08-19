import { useState, type ReactNode } from 'react'

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

export function AppShell({
  children,
  sidebar,
}: {
  children: ReactNode
  sidebar?: ReactNode
}) {
  const [navOpen, setNavOpen] = useState(false)
  const [panelOpen, setPanelOpen] = useState(false)

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

      <div className="lg:grid lg:h-full lg:grid-cols-[16rem_minmax(0,1fr)_0.75rem]">
        <div className="hidden overflow-y-auto border-r border-edge bg-surface lg:block">
          <NavColumn />
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
