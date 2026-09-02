import { describe, it, expect, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";

const purge = vi.fn();
const updateEventBusConfig = vi.fn();
const updateLoggingConfig = vi.fn();

vi.mock("../clients", () => ({
  statsClient: { purge: (...args: unknown[]) => purge(...args) },
  // queries.ts pulls the rest of the clients in at module load; only the
  // hooks exercised below need real mock methods, the others just need to
  // exist.
  agentClient: {},
  configClient: {},
  directoryScannerClient: {},
  eventBusClient: {
    updateEventBusConfig: (...args: unknown[]) => updateEventBusConfig(...args),
  },
  loggingClient: {
    updateLoggingConfig: (...args: unknown[]) => updateLoggingConfig(...args),
  },
  sonarrInterfaceClient: {},
  workflowCatalogClient: {},
  workflowClient: {},
}));

import {
  queryKeys,
  usePurgeStreams,
  useUpdateEventBusConfig,
  useUpdateLoggingConfig,
} from "../queries";

describe("queryKeys", () => {
  describe("static keys", () => {
    it("defines config key", () => {
      expect(queryKeys.config).toEqual(["config"]);
    });

    it("defines nested config keys", () => {
      expect(queryKeys.directoryScanner).toEqual([
        "config",
        "directory-scanner",
      ]);
      expect(queryKeys.scanDirectories).toEqual(["config", "scan-directories"]);
      expect(queryKeys.sidecarTypes).toEqual(["config", "sidecar-types"]);
    });

    it("defines stats keys", () => {
      expect(queryKeys.busSnapshot).toEqual(["stats", "bus"]);
      expect(queryKeys.agents).toEqual(["stats", "agents"]);
      expect(queryKeys.logTail).toEqual(["stats", "log-tail"]);
    });

    it("defines workflow catalog key", () => {
      expect(queryKeys.workflowCatalog).toEqual(["workflows", "catalog"]);
      expect(queryKeys.workflows).toEqual(["workflows"]);
    });
  });

  describe("dynamic keys", () => {
    it("generates workflow keys by id", () => {
      const workflowId = "workflow-456";
      expect(queryKeys.workflow(workflowId)).toEqual(["workflows", workflowId]);
      expect(queryKeys.workflowVersions(workflowId)).toEqual([
        "workflows",
        workflowId,
        "versions",
      ]);
    });

    it("handles different ids independently", () => {
      expect(queryKeys.workflow("id-a")).not.toEqual(
        queryKeys.workflow("id-b"),
      );
    });
  });
});

describe("usePurgeStreams", () => {
  function harness() {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);
    return { queryClient, invalidate, wrapper };
  }

  it("calls StatsService.Purge with a single stream name", async () => {
    purge.mockReset().mockResolvedValue({ results: [] });
    const { wrapper } = harness();

    const { result } = renderHook(() => usePurgeStreams(), { wrapper });
    result.current.mutate({ stream: "events.agent_scan_results" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(purge).toHaveBeenCalledWith({ stream: "events.agent_scan_results" });
  });

  it("calls StatsService.Purge with the all flag", async () => {
    purge.mockReset().mockResolvedValue({ results: [] });
    const { wrapper } = harness();

    const { result } = renderHook(() => usePurgeStreams(), { wrapper });
    result.current.mutate({ all: true });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(purge).toHaveBeenCalledWith({ all: true });
  });

  it("invalidates the bus snapshot on success so the drained depth shows", async () => {
    purge.mockReset().mockResolvedValue({ results: [] });
    const { invalidate, wrapper } = harness();

    const { result } = renderHook(() => usePurgeStreams(), { wrapper });
    result.current.mutate({ stream: "events.agent_scan_results" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: queryKeys.busSnapshot,
    });
  });
});

// The two scalar-section update hooks each send an AIP-134 partial update:
// the changed fields plus an update_mask naming exactly those fields, in the
// lower_snake_case a protobuf FieldMask carries.
function mutationHarness() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const invalidate = vi.spyOn(queryClient, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
  return { invalidate, wrapper };
}

describe("useUpdateEventBusConfig", () => {
  it("calls updateEventBusConfig with a well-formed update_mask and the read etag", async () => {
    updateEventBusConfig
      .mockReset()
      .mockResolvedValue({ name: "operations/c1", done: false });
    const { wrapper } = mutationHarness();

    const { result } = renderHook(() => useUpdateEventBusConfig(), { wrapper });
    result.current.mutate({ patch: { retentionHours: 96 }, etag: "abc123" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateEventBusConfig).toHaveBeenCalledWith({
      config: { retentionHours: 96 },
      updateMask: { paths: ["retention_hours"] },
      etag: "abc123",
    });
  });

  it("maps every changed camelCase key to its snake_case mask path", async () => {
    updateEventBusConfig
      .mockReset()
      .mockResolvedValue({ name: "operations/c2", done: false });
    const { wrapper } = mutationHarness();

    const { result } = renderHook(() => useUpdateEventBusConfig(), { wrapper });
    result.current.mutate({
      patch: { retryBackoffBaseMs: 250, retryBackoffMaxMs: 5000 },
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateEventBusConfig).toHaveBeenCalledWith({
      config: { retryBackoffBaseMs: 250, retryBackoffMaxMs: 5000 },
      updateMask: { paths: ["retry_backoff_base_ms", "retry_backoff_max_ms"] },
      etag: undefined,
    });
  });

  it("invalidates the event-bus and whole-config reads on success", async () => {
    updateEventBusConfig
      .mockReset()
      .mockResolvedValue({ name: "operations/c3", done: false });
    const { invalidate, wrapper } = mutationHarness();

    const { result } = renderHook(() => useUpdateEventBusConfig(), { wrapper });
    result.current.mutate({ patch: { maxLen: 20000 } });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.eventBus });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.config });
  });
});

describe("useUpdateLoggingConfig", () => {
  it("calls updateLoggingConfig with a server_level-only update_mask and the etag", async () => {
    updateLoggingConfig
      .mockReset()
      .mockResolvedValue({ name: "operations/c1", done: false });
    const { wrapper } = mutationHarness();

    const { result } = renderHook(() => useUpdateLoggingConfig(), { wrapper });
    result.current.mutate({ patch: { serverLevel: "debug" }, etag: "log-etag" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateLoggingConfig).toHaveBeenCalledWith({
      config: { serverLevel: "debug" },
      updateMask: { paths: ["server_level"] },
      etag: "log-etag",
    });
  });
});
