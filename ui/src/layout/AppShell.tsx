import { useEffect, useState, type ReactNode } from 'react'
import { Layout, Space, Typography } from 'antd'

import { HoverPinPanel } from './HoverPinPanel'
import { NavColumn } from './NavColumn'
import { Sidebar } from './Sidebar'
import './AppShell.css'

// Persisted across sessions — pinning a panel open is a standing preference,
// not something to re-choose every time the app loads.
const NAV_PINNED_KEY = 'metarr.nav.pinned'
const PANEL_PINNED_KEY = 'metarr.panel.pinned'

// The whole app is one centre panel: no header, no footer. The left nav and
// right panel hide to a thin strip and reveal on hover (or tap, for touch),
// with an independent pin per side to hold either one open — see
// HoverPinPanel for the shared mechanics.
export function AppShell({
  children,
  sidebar,
}: {
  children: ReactNode
  sidebar?: ReactNode
}) {
  const [navPinned, setNavPinned] = useState(
    () => localStorage.getItem(NAV_PINNED_KEY) === 'true',
  )
  const [panelPinned, setPanelPinned] = useState(
    () => localStorage.getItem(PANEL_PINNED_KEY) === 'true',
  )

  useEffect(() => {
    localStorage.setItem(NAV_PINNED_KEY, String(navPinned))
  }, [navPinned])

  useEffect(() => {
    localStorage.setItem(PANEL_PINNED_KEY, String(panelPinned))
  }, [panelPinned])

  return (
    <Layout className="app-shell">
      <HoverPinPanel side="left" pinned={navPinned}>
        <NavColumn pinned={navPinned} onTogglePin={() => setNavPinned((current) => !current)} />
      </HoverPinPanel>

      <Layout.Content className="app-shell-main">{children}</Layout.Content>

      <HoverPinPanel side="right" pinned={panelPinned}>
        <Sidebar pinned={panelPinned} onTogglePin={() => setPanelPinned((current) => !current)}>
          {sidebar}
        </Sidebar>
      </HoverPinPanel>
    </Layout>
  )
}

// PageHeader is the top of the centre column, kept here so every page
// presents its title the same way.
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
    <div className="app-page-header">
      <div>
        <Typography.Title level={4} className="app-page-header-title">
          {title}
        </Typography.Title>
        {description ? (
          <Typography.Text type="secondary" className="app-page-header-description">
            {description}
          </Typography.Text>
        ) : null}
      </div>
      {actions ? <Space>{actions}</Space> : null}
    </div>
  )
}
