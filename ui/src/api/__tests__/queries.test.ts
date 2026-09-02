import { describe, it, expect, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";

const purge = vi.fn();
const getEventBusConfig = vi.fn();
const updateEventBusConfig = vi.fn();
const getLoggingConfig = vi.fn();
const updateLoggingConfig = vi.fn();
const listSonarrInstances = vi.fn();
const createSonarrInstance = vi.fn();
const updateSonarrInstance = vi.fn();
const deleteSonarrInstance = vi.fn();
const createAgent = vi.fn();
const updateAgent = vi.fn();
const deleteAgent = vi.fn();
const setLogLevel = vi.fn();
const updateAdminUser = vi.fn();
const createApiKey = vi.fn();
const updateApiKey = vi.fn();
const deleteApiKey = vi.fn();
const getDirectoryScannerConfig = vi.fn();
const updateDirectoryScannerConfig = vi.fn();
const listScanDirectories = vi.fn();
const createScanDirectory = vi.fn();
const updateScanDirectory = vi.fn();
const deleteScanDirectory = vi.fn();
const listSidecarTypes = vi.fn();
const createSidecarType = vi.fn();
const updateSidecarType = vi.fn();
const deleteSidecarType = vi.fn();
const reorderSidecarTypes = vi.fn();
const resetSidecarTypes = vi.fn();

vi.mock("../clients", () => ({
  statsClient: { purge: (...args: unknown[]) => purge(...args) },
  // queries.ts pulls the rest of the clients in at module load; only the
  // hooks exercised below need real mock methods, the others just need to
  // exist.
  agentClient: {
    createAgent: (...args: unknown[]) => createAgent(...args),
    updateAgent: (...args: unknown[]) => updateAgent(...args),
    deleteAgent: (...args: unknown[]) => deleteAgent(...args),
    setLogLevel: (...args: unknown[]) => setLogLevel(...args),
  },
  configClient: {},
  adminClient: {
    updateAdminUser: (...args: unknown[]) => updateAdminUser(...args),
  },
  apiKeyClient: {
    createApiKey: (...args: unknown[]) => createApiKey(...args),
    updateApiKey: (...args: unknown[]) => updateApiKey(...args),
    deleteApiKey: (...args: unknown[]) => deleteApiKey(...args),
  },
  eventBusClient: {
    getEventBusConfig: (...args: unknown[]) => getEventBusConfig(...args),
    updateEventBusConfig: (...args: unknown[]) => updateEventBusConfig(...args),
  },
  loggingClient: {
    getLoggingConfig: (...args: unknown[]) => getLoggingConfig(...args),
    updateLoggingConfig: (...args: unknown[]) => updateLoggingConfig(...args),
  },
  sonarrInterfaceClient: {
    listSonarrInstances: (...args: unknown[]) => listSonarrInstances(...args),
    createSonarrInstance: (...args: unknown[]) => createSonarrInstance(...args),
    updateSonarrInstance: (...args: unknown[]) => updateSonarrInstance(...args),
    deleteSonarrInstance: (...args: unknown[]) => deleteSonarrInstance(...args),
  },
  directoryScannerClient: {
    getDirectoryScannerConfig: (...args: unknown[]) =>
      getDirectoryScannerConfig(...args),
    updateDirectoryScannerConfig: (...args: unknown[]) =>
      updateDirectoryScannerConfig(...args),
    listScanDirectories: (...args: unknown[]) => listScanDirectories(...args),
    createScanDirectory: (...args: unknown[]) => createScanDirectory(...args),
    updateScanDirectory: (...args: unknown[]) => updateScanDirectory(...args),
    deleteScanDirectory: (...args: unknown[]) => deleteScanDirectory(...args),
    listSidecarTypes: (...args: unknown[]) => listSidecarTypes(...args),
    createSidecarType: (...args: unknown[]) => createSidecarType(...args),
    updateSidecarType: (...args: unknown[]) => updateSidecarType(...args),
    deleteSidecarType: (...args: unknown[]) => deleteSidecarType(...args),
    reorderSidecarTypes: (...args: unknown[]) => reorderSidecarTypes(...args),
    resetSidecarTypes: (...args: unknown[]) => resetSidecarTypes(...args),
  },
  workflowCatalogClient: {},
  workflowClient: {},
}));

import {
  queryKeys,
  usePurgeStreams,
  useEventBusConfig,
  useLoggingConfig,
  useUpdateEventBusConfig,
  useUpdateLoggingConfig,
  useSonarrInstances,
  useCreateSonarrInstance,
  useUpdateSonarrInstance,
  useDeleteSonarrInstance,
  useScanDirectories,
  useUpdateDirectoryScanner,
  useCreateScanDirectory,
  useUpdateScanDirectory,
  useDeleteScanDirectory,
  useSidecarTypes,
  useCreateSidecarType,
  useUpdateSidecarType,
  useDeleteSidecarType,
  useReorderSidecarTypes,
  useResetSidecarTypes,
  useCreateAgent,
  useUpdateAgent,
  useDeleteAgent,
  useSetAgentLogLevel,
  useUpdateAdmin,
  useCreateApiKey,
  useUpdateApiKey,
  useDeleteApiKey,
} from "../queries";
import { AccessLevel } from "../../gen/metarr/v1/api_keys_pb";

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
// lower_snake_case a protobuf FieldMask carries. The write is synchronous and
// returns the stored section.
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
  return { queryClient, invalidate, wrapper };
}

describe("useUpdateEventBusConfig", () => {
  it("calls updateEventBusConfig with a well-formed update_mask", async () => {
    updateEventBusConfig.mockReset().mockResolvedValue({ retentionHours: 96 });
    const { wrapper } = mutationHarness();

    const { result } = renderHook(() => useUpdateEventBusConfig(), { wrapper });
    result.current.mutate({ patch: { retentionHours: 96 } });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateEventBusConfig).toHaveBeenCalledWith({
      config: { retentionHours: 96 },
      updateMask: { paths: ["retention_hours"] },
    });
  });

  it("maps every changed camelCase key to its snake_case mask path", async () => {
    updateEventBusConfig
      .mockReset()
      .mockResolvedValue({ retryBackoffBaseMs: 250, retryBackoffMaxMs: 5000 });
    const { wrapper } = mutationHarness();

    const { result } = renderHook(() => useUpdateEventBusConfig(), { wrapper });
    result.current.mutate({
      patch: { retryBackoffBaseMs: 250, retryBackoffMaxMs: 5000 },
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateEventBusConfig).toHaveBeenCalledWith({
      config: { retryBackoffBaseMs: 250, retryBackoffMaxMs: 5000 },
      updateMask: { paths: ["retry_backoff_base_ms", "retry_backoff_max_ms"] },
    });
  });

  it("writes the returned section into the event-bus cache and does not refetch it", async () => {
    const stored = {
      maxLen: 20000,
      retentionHours: 48,
      retryAttempts: 4,
      retryBackoffBaseMs: 500,
      retryBackoffMaxMs: 30000,
    };
    getEventBusConfig.mockReset().mockResolvedValue({
      config: { maxLen: 10000, retentionHours: 48 },
    });
    updateEventBusConfig.mockReset().mockResolvedValue(stored);
    const { queryClient, invalidate, wrapper } = mutationHarness();

    // The section read is mounted, as it is on the Event Bus screen: a naive
    // invalidate({queryKey: ["config"]}) would fuzzy-match ["config","event-bus"]
    // and refetch it, undoing the setQueryData below.
    const { result } = renderHook(
      () => ({
        read: useEventBusConfig(),
        update: useUpdateEventBusConfig(),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.read.isSuccess).toBe(true));
    expect(getEventBusConfig).toHaveBeenCalledTimes(1);

    result.current.update.mutate({ patch: { maxLen: 20000 } });
    await waitFor(() => expect(result.current.update.isSuccess).toBe(true));

    // The synchronous write is authoritative: its response lands in the cache
    // directly and no GET round-trip runs — no queued→confirmed poll.
    expect(queryClient.getQueryData(queryKeys.eventBus)).toEqual(stored);
    expect(getEventBusConfig).toHaveBeenCalledTimes(1);
    // Only the sibling aggregate GetConfig read is invalidated, exact so the
    // fuzzy match cannot sweep the section read back in.
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: queryKeys.config,
      exact: true,
    });
  });
});

describe("useUpdateLoggingConfig", () => {
  it("calls updateLoggingConfig with a server_level-only update_mask", async () => {
    updateLoggingConfig.mockReset().mockResolvedValue({ serverLevel: "debug" });
    const { wrapper } = mutationHarness();

    const { result } = renderHook(() => useUpdateLoggingConfig(), { wrapper });
    result.current.mutate({ patch: { serverLevel: "debug" } });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateLoggingConfig).toHaveBeenCalledWith({
      config: { serverLevel: "debug" },
      updateMask: { paths: ["server_level"] },
    });
  });

  it("writes the returned section into the logging cache and does not refetch it", async () => {
    const stored = {
      serverLevel: "debug",
      sink: "fluent-bit",
      endpoint: "http://openobserve.example/logs",
      stream: "metarr",
    };
    getLoggingConfig.mockReset().mockResolvedValue({
      config: { serverLevel: "info", sink: "fluent-bit" },
    });
    updateLoggingConfig.mockReset().mockResolvedValue(stored);
    const { queryClient, invalidate, wrapper } = mutationHarness();

    const { result } = renderHook(
      () => ({
        read: useLoggingConfig(),
        update: useUpdateLoggingConfig(),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.read.isSuccess).toBe(true));
    expect(getLoggingConfig).toHaveBeenCalledTimes(1);

    result.current.update.mutate({ patch: { serverLevel: "debug" } });
    await waitFor(() => expect(result.current.update.isSuccess).toBe(true));

    expect(queryClient.getQueryData(queryKeys.logging)).toEqual(stored);
    expect(getLoggingConfig).toHaveBeenCalledTimes(1);
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: queryKeys.config,
      exact: true,
    });
  });
});

// SonarrInterfaceService is a collection on AIP standard methods: Create /
// Update return the stored instance, Delete returns empty, and each hook
// splices its result into the ["config","interfaces","sonarr"] cache rather
// than refetching the list.
describe("Sonarr instance hooks", () => {
  const instanceA = {
    instanceSlug: "sonarr-a",
    instanceName: "A",
    sonarrUrl: "http://a:8989",
    sonarrApiKey: "key-a",
    rootDirMap: [],
    storage: { mode: "cache", ttl: "24h", maxCount: 0 },
  };

  it("useSonarrInstances drains the paginated list", async () => {
    const instanceB = {
      ...instanceA,
      instanceSlug: "sonarr-b",
      instanceName: "B",
    };
    listSonarrInstances
      .mockReset()
      .mockResolvedValueOnce({
        sonarrInstances: [instanceA],
        nextPageToken: "page-2",
      })
      .mockResolvedValueOnce({
        sonarrInstances: [instanceB],
        nextPageToken: "",
      });
    const { wrapper } = mutationHarness();

    const { result } = renderHook(() => useSonarrInstances(), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(listSonarrInstances).toHaveBeenNthCalledWith(1, { pageToken: "" });
    expect(listSonarrInstances).toHaveBeenNthCalledWith(2, {
      pageToken: "page-2",
    });
    expect(result.current.data).toEqual([instanceA, instanceB]);
  });

  it("useCreateSonarrInstance sends the slug in sonarr_instance_id and appends the stored instance", async () => {
    createSonarrInstance.mockReset().mockResolvedValue(instanceA);
    const { queryClient, invalidate, wrapper } = mutationHarness();
    queryClient.setQueryData(queryKeys.sonarr, []);

    const { result } = renderHook(() => useCreateSonarrInstance(), { wrapper });
    result.current.mutate({
      instanceSlug: "sonarr-a",
      instanceName: "A",
      sonarrUrl: "http://a:8989",
      sonarrApiKey: "key-a",
      rootDirMap: [],
      storage: { mode: "cache", ttl: "24h", maxCount: 0 },
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(createSonarrInstance).toHaveBeenCalledWith({
      sonarrInstanceId: "sonarr-a",
      sonarrInstance: expect.objectContaining({ instanceSlug: "sonarr-a" }),
    });
    expect(queryClient.getQueryData(queryKeys.sonarr)).toEqual([instanceA]);
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: queryKeys.config,
      exact: true,
    });
  });

  it("useUpdateSonarrInstance sends a full field mask and replaces the cached entry", async () => {
    const updated = { ...instanceA, instanceName: "A renamed" };
    updateSonarrInstance.mockReset().mockResolvedValue(updated);
    const { queryClient, wrapper } = mutationHarness();
    queryClient.setQueryData(queryKeys.sonarr, [instanceA]);

    const { result } = renderHook(() => useUpdateSonarrInstance(), { wrapper });
    result.current.mutate({ ...instanceA, instanceName: "A renamed" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateSonarrInstance).toHaveBeenCalledWith({
      sonarrInstance: expect.objectContaining({ instanceSlug: "sonarr-a" }),
      updateMask: {
        paths: [
          "instance_name",
          "sonarr_url",
          "sonarr_api_key",
          "root_dir_map",
          "storage",
        ],
      },
    });
    expect(queryClient.getQueryData(queryKeys.sonarr)).toEqual([updated]);
  });

  it("useDeleteSonarrInstance sends the slug and drops the cached entry", async () => {
    deleteSonarrInstance.mockReset().mockResolvedValue({});
    const { queryClient, invalidate, wrapper } = mutationHarness();
    queryClient.setQueryData(queryKeys.sonarr, [instanceA]);

    const { result } = renderHook(() => useDeleteSonarrInstance(), { wrapper });
    result.current.mutate("sonarr-a");

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(deleteSonarrInstance).toHaveBeenCalledWith({ slug: "sonarr-a" });
    expect(queryClient.getQueryData(queryKeys.sonarr)).toEqual([]);
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: queryKeys.config,
      exact: true,
    });
  });
});

// DirectoryScannerService is on AIP standard methods: the scalar section's
// UpdateDirectoryScannerConfig and every sub-collection write is synchronous
// and returns the stored resource, which each hook splices into its section
// cache rather than refetching.
describe("Directory scanner hooks", () => {
  const dirA = { scannerSlug: "movies", scanType: "movie", directory: "/m" };
  const typeA = {
    id: "t-1",
    type: "poster",
    category: "image",
    order: 10,
    patterns: ["(?i)^poster$"],
    extensions: [],
  };

  it("useScanDirectories drains the paginated list", async () => {
    const dirB = { ...dirA, scannerSlug: "tv", directory: "/t" };
    listScanDirectories
      .mockReset()
      .mockResolvedValueOnce({ scanDirectories: [dirA], nextPageToken: "p2" })
      .mockResolvedValueOnce({ scanDirectories: [dirB], nextPageToken: "" });
    const { wrapper } = mutationHarness();

    const { result } = renderHook(() => useScanDirectories(), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(listScanDirectories).toHaveBeenNthCalledWith(1, { pageToken: "" });
    expect(listScanDirectories).toHaveBeenNthCalledWith(2, { pageToken: "p2" });
    expect(result.current.data).toEqual([dirA, dirB]);
  });

  it("useSidecarTypes drains the paginated list", async () => {
    listSidecarTypes
      .mockReset()
      .mockResolvedValueOnce({ sidecarTypes: [typeA], nextPageToken: "" });
    const { wrapper } = mutationHarness();

    const { result } = renderHook(() => useSidecarTypes(), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([typeA]);
  });

  it("useUpdateDirectoryScanner sends a parallel_count mask and caches the stored section", async () => {
    const stored = { parallelCount: 8, scanDirectories: [], sidecarTypes: [] };
    updateDirectoryScannerConfig.mockReset().mockResolvedValue(stored);
    const { queryClient, invalidate, wrapper } = mutationHarness();

    const { result } = renderHook(() => useUpdateDirectoryScanner(), {
      wrapper,
    });
    result.current.mutate({ parallelCount: 8 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateDirectoryScannerConfig).toHaveBeenCalledWith({
      config: { parallelCount: 8 },
      updateMask: { paths: ["parallel_count"] },
    });
    expect(queryClient.getQueryData(queryKeys.directoryScanner)).toEqual(
      stored,
    );
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: queryKeys.config,
      exact: true,
    });
  });

  it("useCreateScanDirectory sends the slug in scan_directory_id and appends", async () => {
    createScanDirectory.mockReset().mockResolvedValue(dirA);
    const { queryClient, wrapper } = mutationHarness();
    queryClient.setQueryData(queryKeys.scanDirectories, []);

    const { result } = renderHook(() => useCreateScanDirectory(), { wrapper });
    result.current.mutate(dirA);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(createScanDirectory).toHaveBeenCalledWith({
      scanDirectoryId: "movies",
      scanDirectory: dirA,
    });
    expect(queryClient.getQueryData(queryKeys.scanDirectories)).toEqual([dirA]);
  });

  it("useUpdateScanDirectory sends the writable-field mask and replaces the cached entry", async () => {
    const updated = { ...dirA, directory: "/movies-4k" };
    updateScanDirectory.mockReset().mockResolvedValue(updated);
    const { queryClient, wrapper } = mutationHarness();
    queryClient.setQueryData(queryKeys.scanDirectories, [dirA]);

    const { result } = renderHook(() => useUpdateScanDirectory(), { wrapper });
    result.current.mutate(updated);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateScanDirectory).toHaveBeenCalledWith({
      scanDirectory: updated,
      updateMask: { paths: ["scan_type", "directory"] },
    });
    expect(queryClient.getQueryData(queryKeys.scanDirectories)).toEqual([
      updated,
    ]);
  });

  it("useDeleteScanDirectory sends the slug and drops the cached entry", async () => {
    deleteScanDirectory.mockReset().mockResolvedValue({});
    const { queryClient, wrapper } = mutationHarness();
    queryClient.setQueryData(queryKeys.scanDirectories, [dirA]);

    const { result } = renderHook(() => useDeleteScanDirectory(), { wrapper });
    result.current.mutate("movies");

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(deleteScanDirectory).toHaveBeenCalledWith({ slug: "movies" });
    expect(queryClient.getQueryData(queryKeys.scanDirectories)).toEqual([]);
  });

  it("useCreateSidecarType sends the resource with no id and appends", async () => {
    createSidecarType.mockReset().mockResolvedValue(typeA);
    const { queryClient, wrapper } = mutationHarness();
    queryClient.setQueryData(queryKeys.sidecarTypes, []);

    const { result } = renderHook(() => useCreateSidecarType(), { wrapper });
    result.current.mutate({
      id: "",
      type: "poster",
      category: "image",
      order: 0,
      patterns: ["(?i)^poster$"],
      extensions: [],
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(createSidecarType).toHaveBeenCalledWith({
      sidecarType: expect.objectContaining({ type: "poster", id: "" }),
    });
    expect(queryClient.getQueryData(queryKeys.sidecarTypes)).toEqual([typeA]);
  });

  it("useUpdateSidecarType sends the writable-field mask and replaces the cached entry", async () => {
    const updated = { ...typeA, category: "subtitle" };
    updateSidecarType.mockReset().mockResolvedValue(updated);
    const { queryClient, wrapper } = mutationHarness();
    queryClient.setQueryData(queryKeys.sidecarTypes, [typeA]);

    const { result } = renderHook(() => useUpdateSidecarType(), { wrapper });
    result.current.mutate(updated);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateSidecarType).toHaveBeenCalledWith({
      sidecarType: updated,
      updateMask: { paths: ["type", "category", "patterns", "extensions"] },
    });
    expect(queryClient.getQueryData(queryKeys.sidecarTypes)).toEqual([updated]);
  });

  it("useDeleteSidecarType sends the id and drops the cached entry", async () => {
    deleteSidecarType.mockReset().mockResolvedValue({});
    const { queryClient, wrapper } = mutationHarness();
    queryClient.setQueryData(queryKeys.sidecarTypes, [typeA]);

    const { result } = renderHook(() => useDeleteSidecarType(), { wrapper });
    result.current.mutate("t-1");

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(deleteSidecarType).toHaveBeenCalledWith({ id: "t-1" });
    expect(queryClient.getQueryData(queryKeys.sidecarTypes)).toEqual([]);
  });

  it("useReorderSidecarTypes writes the returned list into the section cache", async () => {
    const reordered = [{ ...typeA, order: 20 }];
    reorderSidecarTypes
      .mockReset()
      .mockResolvedValue({ sidecarTypes: reordered });
    const { queryClient, invalidate, wrapper } = mutationHarness();
    queryClient.setQueryData(queryKeys.sidecarTypes, [typeA]);

    const { result } = renderHook(() => useReorderSidecarTypes(), { wrapper });
    result.current.mutate({ "t-1": 20 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(reorderSidecarTypes).toHaveBeenCalledWith({ orders: { "t-1": 20 } });
    expect(queryClient.getQueryData(queryKeys.sidecarTypes)).toEqual(reordered);
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: queryKeys.config,
      exact: true,
    });
  });

  it("useResetSidecarTypes writes the returned defaults into the section cache", async () => {
    const defaults = [typeA];
    resetSidecarTypes.mockReset().mockResolvedValue({ sidecarTypes: defaults });
    const { queryClient, wrapper } = mutationHarness();
    queryClient.setQueryData(queryKeys.sidecarTypes, []);

    const { result } = renderHook(() => useResetSidecarTypes(), { wrapper });
    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(resetSidecarTypes).toHaveBeenCalledWith({});
    expect(queryClient.getQueryData(queryKeys.sidecarTypes)).toEqual(defaults);
  });
});

// AgentService is a slug-addressed collection on AIP standard methods. The
// agents cache (["stats","agents"]) is socket-fed by StreamPresence, so each
// write invalidates it (the refetch re-merges live presence) and the sibling
// aggregate GetConfig read (exact), rather than splicing a presence-less
// response into it.
describe("Agent hooks", () => {
  const agent = {
    slug: "nas-01",
    displayName: "NAS",
    mappings: [{ scannerSlug: "movies", agentPath: "/mnt/movies" }],
  };

  it("useCreateAgent sends the slug in agent_id and invalidates", async () => {
    createAgent.mockReset().mockResolvedValue({ ...agent, configured: true });
    const { invalidate, wrapper } = mutationHarness();

    const { result } = renderHook(() => useCreateAgent(), { wrapper });
    result.current.mutate(agent);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(createAgent).toHaveBeenCalledWith({
      agentId: "nas-01",
      agent,
    });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.agents });
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: queryKeys.config,
      exact: true,
    });
  });

  it("useUpdateAgent sends the writable-field mask", async () => {
    updateAgent.mockReset().mockResolvedValue({ ...agent, configured: true });
    const { wrapper } = mutationHarness();

    const { result } = renderHook(() => useUpdateAgent(), { wrapper });
    result.current.mutate({ ...agent, displayName: "Renamed" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateAgent).toHaveBeenCalledWith({
      agent: { ...agent, displayName: "Renamed" },
      updateMask: { paths: ["display_name", "mappings"] },
    });
  });

  it("useDeleteAgent sends the slug and invalidates", async () => {
    deleteAgent.mockReset().mockResolvedValue({});
    const { invalidate, wrapper } = mutationHarness();

    const { result } = renderHook(() => useDeleteAgent(), { wrapper });
    result.current.mutate("nas-01");

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(deleteAgent).toHaveBeenCalledWith({ slug: "nas-01" });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.agents });
  });

  it("useSetAgentLogLevel maps log_level to logLevel", async () => {
    setLogLevel.mockReset().mockResolvedValue({ ...agent, logLevel: "debug" });
    const { invalidate, wrapper } = mutationHarness();

    const { result } = renderHook(() => useSetAgentLogLevel(), { wrapper });
    result.current.mutate({ slug: "nas-01", log_level: "debug" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(setLogLevel).toHaveBeenCalledWith({
      slug: "nas-01",
      logLevel: "debug",
    });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.agents });
  });
});

// AdminService.UpdateAdminUser is an AIP-134 partial update: the hook derives
// the update_mask from which identity fields the caller passed, and a new
// password rides new_password (never the mask). Each variant invalidates the
// aggregate GetConfig read the Security screen paints from.
describe("useUpdateAdmin", () => {
  it("sends a username-only mask and an empty new_password", async () => {
    updateAdminUser.mockReset().mockResolvedValue({ username: "renamed" });
    const { invalidate, wrapper } = mutationHarness();

    const { result } = renderHook(() => useUpdateAdmin(), { wrapper });
    result.current.mutate({ username: "renamed" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateAdminUser).toHaveBeenCalledWith({
      admin: { username: "renamed" },
      updateMask: { paths: ["username"] },
      newPassword: "",
    });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.config });
  });

  it("sends an empty mask and the new password when only a password changes", async () => {
    updateAdminUser.mockReset().mockResolvedValue({ username: "admin" });
    const { wrapper } = mutationHarness();

    const { result } = renderHook(() => useUpdateAdmin(), { wrapper });
    result.current.mutate({ password: "a-new-secret" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateAdminUser).toHaveBeenCalledWith({
      admin: {},
      updateMask: { paths: [] },
      newPassword: "a-new-secret",
    });
  });
});

// ApiKeyService is a minted-id collection scoped by the AccessLevel enum.
describe("API key hooks", () => {
  it("useCreateApiKey sends the access level and a name, no id", async () => {
    createApiKey.mockReset().mockResolvedValue({ id: "minted", name: "ci" });
    const { invalidate, wrapper } = mutationHarness();

    const { result } = renderHook(() => useCreateApiKey(), { wrapper });
    result.current.mutate({ accessLevel: AccessLevel.WEBHOOK, name: "ci" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(createApiKey).toHaveBeenCalledWith({
      accessLevel: AccessLevel.WEBHOOK,
      apiKey: { name: "ci" },
    });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.config });
  });

  it("useUpdateApiKey derives the mask from the changed field", async () => {
    updateApiKey.mockReset().mockResolvedValue({ id: "k1", apiKey: "v" });
    const { wrapper } = mutationHarness();

    const { result } = renderHook(() => useUpdateApiKey(), { wrapper });
    result.current.mutate({ id: "k1", apiKey: "v" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(updateApiKey).toHaveBeenCalledWith({
      apiKey: { id: "k1", apiKey: "v" },
      updateMask: { paths: ["api_key"] },
    });
  });

  it("useDeleteApiKey sends the id alone", async () => {
    deleteApiKey.mockReset().mockResolvedValue({});
    const { invalidate, wrapper } = mutationHarness();

    const { result } = renderHook(() => useDeleteApiKey(), { wrapper });
    result.current.mutate("k1");

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(deleteApiKey).toHaveBeenCalledWith({ id: "k1" });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.config });
  });
});
