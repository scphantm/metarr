import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { render, screen } from "@testing-library/react";

import { App } from "./App";
import { AuthenticationScheme } from "./gen/metarr/v1/admin_pb";

// The render gate is the unit under test; the routed shell, the login
// screen, and the startup placeholder are stubbed to bare markers so each
// assertion is unambiguous.
const getAuthScheme = vi.fn();

vi.mock("./api/clients", () => ({
  authClient: { getAuthScheme: (...args: unknown[]) => getAuthScheme(...args) },
  // queries.ts pulls the rest in at module load; they only need to exist.
  adminClient: {},
  agentClient: {},
  configClient: {},
  directoryScannerClient: {},
  eventBusClient: {},
  loggingClient: {},
  sonarrInterfaceClient: {},
  statsClient: {},
  workflowCatalogClient: {},
  workflowClient: {},
}));

let isAuthenticated = false;
vi.mock("./auth/AuthContext", () => ({
  useAuth: () => ({ isAuthenticated }),
}));

vi.mock("./auth/LoginScreen", () => ({
  LoginScreen: () => <div data-testid="login-screen" />,
}));
vi.mock("./auth/StartupGate", () => ({
  StartupGate: () => <div data-testid="startup-gate" />,
}));
vi.mock("react-router-dom", async (importActual) => {
  const actual = await importActual<typeof import("react-router-dom")>();
  return {
    ...actual,
    Routes: () => <div data-testid="app-shell" />,
    Route: () => null,
    Navigate: () => null,
  };
});

function renderApp() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("App render gate", () => {
  beforeEach(() => {
    getAuthScheme.mockReset();
    isAuthenticated = false;
  });

  it("shows the deterministic startup state while the scheme probe is in flight", () => {
    getAuthScheme.mockReturnValue(new Promise<never>(() => {}));

    renderApp();

    expect(screen.getByTestId("startup-gate")).toBeDefined();
    expect(screen.queryByTestId("login-screen")).toBeNull();
    expect(screen.queryByTestId("app-shell")).toBeNull();
  });

  it("renders the app shell with no login screen when the scheme is None", async () => {
    getAuthScheme.mockResolvedValue({ scheme: AuthenticationScheme.NONE });

    renderApp();

    expect(await screen.findByTestId("app-shell")).toBeDefined();
    expect(screen.queryByTestId("login-screen")).toBeNull();
  });

  it("renders the login screen when the scheme is Password and no session key is held", async () => {
    getAuthScheme.mockResolvedValue({ scheme: AuthenticationScheme.PASSWORD });

    renderApp();

    expect(await screen.findByTestId("login-screen")).toBeDefined();
    expect(screen.queryByTestId("app-shell")).toBeNull();
  });

  it("renders the app shell when the scheme is Password but a session key is held", async () => {
    getAuthScheme.mockResolvedValue({ scheme: AuthenticationScheme.PASSWORD });
    isAuthenticated = true;

    renderApp();

    expect(await screen.findByTestId("app-shell")).toBeDefined();
    expect(screen.queryByTestId("login-screen")).toBeNull();
  });
});
