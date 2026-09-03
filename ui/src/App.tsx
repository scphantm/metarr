import { Navigate, Route, Routes } from "react-router-dom";

import { useAuthScheme } from "./api/queries";
import { AuthenticationScheme } from "./gen/metarr/v1/admin_pb";
import { useAuth } from "./auth/AuthContext";
import { LoginScreen } from "./auth/LoginScreen";
import { StartupGate } from "./auth/StartupGate";
import { AppShell } from "./layout/AppShell";
import { PageContextProvider } from "./pagecontext/PageContextRegistry";
import { AgentsPage, AgentsSidebar } from "./pages/system/AgentsPage";
import { LoggingPage, LoggingSidebar } from "./pages/system/LoggingPage";
import { ConfigurationPage } from "./pages/system/ConfigurationPage";
import {
  DirectoryScannerPage,
  DirectoryScannerSidebar,
} from "./pages/system/DirectoryScannerPage";
import { EventBusPage, EventBusSidebar } from "./pages/system/EventBusPage";
import {
  ExternalToolsPage,
  ExternalToolsSidebar,
} from "./pages/system/ExternalToolsPage";
import {
  InterfacesPage,
  InterfacesSidebar,
} from "./pages/system/InterfacesPage";
import { SecurityPage, SecuritySidebar } from "./pages/system/SecurityPage";
import { SidecarsPage, SidecarsSidebar } from "./pages/system/SidecarsPage";
import {
  SystemDashboardPage,
  SystemDashboardSidebar,
} from "./pages/system/SystemDashboardPage";
import { WorkflowEditorPage } from "./pages/workflows/WorkflowEditorPage";
import { WorkflowListPage } from "./pages/workflows/WorkflowListPage";

export function App() {
  const { isAuthenticated } = useAuth();
  const authScheme = useAuthScheme();

  // Resolve the scheme before the first gate decision so a cold load never
  // flashes the app shell on the way to the login screen (docs/adr/0012).
  if (authScheme.isLoading) {
    return <StartupGate />;
  }

  // Fail closed: show the login screen unless the probe positively reported
  // scheme None. An unreachable server (data undefined) keeps today's
  // behaviour — the login wall.
  const schemeAllowsOpenAccess = authScheme.data === AuthenticationScheme.NONE;
  if (!schemeAllowsOpenAccess && !isAuthenticated) {
    return <LoginScreen />;
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
          path="/system/event-bus"
          element={
            <AppShell sidebar={<EventBusSidebar />}>
              <EventBusPage />
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
    </PageContextProvider>
  );
}
