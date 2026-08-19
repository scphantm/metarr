import { Navigate, Route, Routes } from 'react-router-dom'

import { useAuth } from './auth/AuthContext'
import { LoginScreen } from './auth/LoginScreen'
import { AppShell } from './layout/AppShell'
import { AgentsPage, AgentsSidebar } from './pages/system/AgentsPage'
import { ConfigurationPage } from './pages/system/ConfigurationPage'
import {
  DirectoryScannerPage,
  DirectoryScannerSidebar,
} from './pages/system/DirectoryScannerPage'
import { InterfacesPage, InterfacesSidebar } from './pages/system/InterfacesPage'
import { SecurityPage, SecuritySidebar } from './pages/system/SecurityPage'
import { SidecarsPage, SidecarsSidebar } from './pages/system/SidecarsPage'
import {
  SystemDashboardPage,
  SystemDashboardSidebar,
} from './pages/system/SystemDashboardPage'

export function App() {
  const { isAuthenticated } = useAuth()

  if (!isAuthenticated) {
    return <LoginScreen />
  }

  return (
    <Routes>
      <Route
        path="/system"
        element={
          <AppShell sidebar={<SystemDashboardSidebar />}>
            <SystemDashboardPage />
          </AppShell>
        }
      />
      <Route
        path="/system/configuration"
        element={
          <AppShell>
            <ConfigurationPage />
          </AppShell>
        }
      />
      <Route
        path="/system/directory-scanner"
        element={
          <AppShell sidebar={<DirectoryScannerSidebar />}>
            <DirectoryScannerPage />
          </AppShell>
        }
      />
      <Route
        path="/system/sidecars"
        element={
          <AppShell sidebar={<SidecarsSidebar />}>
            <SidecarsPage />
          </AppShell>
        }
      />
      <Route
        path="/system/interfaces"
        element={
          <AppShell sidebar={<InterfacesSidebar />}>
            <InterfacesPage />
          </AppShell>
        }
      />
      <Route
        path="/system/agents"
        element={
          <AppShell sidebar={<AgentsSidebar />}>
            <AgentsPage />
          </AppShell>
        }
      />
      <Route
        path="/system/security"
        element={
          <AppShell sidebar={<SecuritySidebar />}>
            <SecurityPage />
          </AppShell>
        }
      />
      {/* The system dashboard is the landing screen; anything unrecognised
          goes there. */}
      <Route path="*" element={<Navigate to="/system" replace />} />
    </Routes>
  )
}
