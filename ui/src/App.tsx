import { Navigate, Route, Routes } from 'react-router-dom'

import { useAuth } from './auth/AuthContext'
import { LoginScreen } from './auth/LoginScreen'
import { ChatWidget } from './chatbot/ChatWidget'
import { AppShell } from './layout/AppShell'
import { PageContextProvider } from './pagecontext/PageContextRegistry'
import { AgentsPage, AgentsSidebar } from './pages/system/AgentsPage'
import { ChatbotSettingsPage, ChatbotSidebar } from './pages/system/ChatbotSettingsPage'
import { LoggingPage, LoggingSidebar } from './pages/system/LoggingPage'
import { ConfigurationPage } from './pages/system/ConfigurationPage'
import {
  DirectoryScannerPage,
  DirectoryScannerSidebar,
} from './pages/system/DirectoryScannerPage'
import {
  ExternalToolsPage,
  ExternalToolsSidebar,
} from './pages/system/ExternalToolsPage'
import { InterfacesPage, InterfacesSidebar } from './pages/system/InterfacesPage'
import { SecurityPage, SecuritySidebar } from './pages/system/SecurityPage'
import { SidecarsPage, SidecarsSidebar } from './pages/system/SidecarsPage'
import {
  SystemDashboardPage,
  SystemDashboardSidebar,
} from './pages/system/SystemDashboardPage'
import { WorkflowEditorPage } from './pages/workflows/WorkflowEditorPage'
import { WorkflowListPage } from './pages/workflows/WorkflowListPage'

export function App() {
  const { isAuthenticated } = useAuth()

  if (!isAuthenticated) {
    return <LoginScreen />
  }

  return (
    <PageContextProvider>
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
          path="/system/logging"
          element={
            <AppShell sidebar={<LoggingSidebar />}>
              <LoggingPage />
            </AppShell>
          }
        />
        <Route
          path="/system/chatbot"
          element={
            <AppShell sidebar={<ChatbotSidebar />}>
              <ChatbotSettingsPage />
            </AppShell>
          }
        />
        <Route
          path="/system/external-tools"
          element={
            <AppShell sidebar={<ExternalToolsSidebar />}>
              <ExternalToolsPage />
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
        <Route
          path="/workflows"
          element={
            <AppShell>
              <WorkflowListPage />
            </AppShell>
          }
        />
        <Route
          path="/workflows/add"
          element={
            <AppShell>
              <WorkflowEditorPage />
            </AppShell>
          }
        />
        <Route
          path="/workflows/:id/edit"
          element={
            <AppShell>
              <WorkflowEditorPage />
            </AppShell>
          }
        />
        {/* The system dashboard is the landing screen; anything unrecognised
            goes there. */}
        <Route path="*" element={<Navigate to="/system" replace />} />
      </Routes>
      <ChatWidget />
    </PageContextProvider>
  )
}
