import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
  type UseMutationResult,
} from "@tanstack/react-query";

import {
  agentClient,
  configClient,
  directoryScannerClient,
  eventBusClient,
  loggingClient,
  sonarrInterfaceClient,
  statsClient,
  workflowCatalogClient,
  workflowClient,
} from "./clients";
import {
  mapAsync,
  registerStream,
  Stream,
  useStream,
  useStreamStatus,
} from "./streams";
import type { DescMessage, MessageInitShape } from "@bufbuild/protobuf";
import type { AcceptedResponse as ConnectAcceptedResponse } from "../gen/metarr/v1/common_pb";
import type { EventBusConfig as ConnectEventBusConfig } from "../gen/metarr/v1/event_bus_pb";
import type { LoggingConfig as ConnectLoggingConfig } from "../gen/metarr/v1/logging_pb";
import {
  SonarrInstanceSchema,
  type SonarrInstance as ConnectSonarrInstance,
} from "../gen/metarr/v1/sonarr_interfaces_pb";
import {
  ConfigServiceDeleteApiKeyRequestSchema,
  ConfigServiceUpdateAdminRequestSchema,
  ConfigServiceUpsertApiKeyRequestSchema,
} from "../gen/metarr/v1/config_pb";
import { AgentConfigSchema } from "../gen/metarr/v1/agents_pb";
import {
  SidecarTypeDefinitionSchema,
  type SidecarTypeDefinition as ConnectSidecarType,
} from "../gen/metarr/bus/v1/agent_contract_pb";
import {
  ScanDirectorySchema,
  type ScanDirectory as ConnectScanDirectory,
  type DirectoryScannerConfig as ConnectDirectoryScannerConfig,
} from "../gen/metarr/v1/directory_scanner_pb";
import { LoggingConfigSchema } from "../gen/metarr/v1/logging_pb";
import { EventBusConfigSchema } from "../gen/metarr/v1/event_bus_pb";
import { WorkflowServiceUpsertRequestSchema } from "../gen/metarr/v1/workflows_pb";

export const queryKeys = {
  config: ["config"] as const,
  directoryScanner: ["config", "directory-scanner"] as const,
  scanDirectories: ["config", "scan-directories"] as const,
  sidecarTypes: ["config", "sidecar-types"] as const,
  sonarr: ["config", "interfaces", "sonarr"] as const,
  // Deliberately outside the config tree: these are fed by a socket, and the
  // config-wide invalidations should not reach them.
  busSnapshot: ["stats", "bus"] as const,
  agents: ["stats", "agents"] as const,
  logging: ["config", "logging"] as const,
  eventBus: ["config", "event-bus"] as const,
  logTail: ["stats", "log-tail"] as const,
  // Outside the config tree: workflows are a server-only, single-
  // collection concern with no config-mutation event behind them at all.
  workflows: ["workflows"] as const,
  workflow: (id: string) => ["workflows", id] as const,
  workflowVersions: (id: string) => ["workflows", id, "versions"] as const,
  workflowCatalog: ["workflows", "catalog"] as const,
};

/*
 * Reads.
 *
 * The still-async config sections write through a system_config_update event,
 * so a mutation settling does not mean the read is fresh yet: those mutations
 * invalidate their queries and the sections that own them poll briefly while a
 * save is outstanding — see useSaveState. The scalar sections on the
 * synchronous AIP write path (event bus, logging — docs/adr/0002) are the
 * exception: their Update returns the stored section, which is written straight
 * into the section's cache, so no poll runs.
 */

export function useConfig() {
  return useQuery({
    queryKey: queryKeys.config,
    queryFn: async () => (await configClient.get({})).config,
  });
}

export function useDirectoryScannerConfig() {
  return useQuery({
    queryKey: queryKeys.directoryScanner,
    queryFn: async () =>
      (await directoryScannerClient.getDirectoryScannerConfig({})).config,
  });
}

// ListScanDirectories / ListSidecarTypes are paginated (AIP-158) with a
// server-side page cap, so — like useSonarrInstances — the settings screens
// drain the pages rather than assuming one call returns them all. Both
// collections are operator-bounded, so this is one page in practice.
export function useScanDirectories() {
  return useQuery({
    queryKey: queryKeys.scanDirectories,
    queryFn: async () => {
      const directories = [];
      let pageToken = "";
      do {
        const page = await directoryScannerClient.listScanDirectories({
          pageToken,
        });
        directories.push(...page.scanDirectories);
        pageToken = page.nextPageToken;
      } while (pageToken !== "");
      return directories;
    },
  });
}

export function useSidecarTypes() {
  return useQuery({
    queryKey: queryKeys.sidecarTypes,
    queryFn: async () => {
      const types = [];
      let pageToken = "";
      do {
        const page = await directoryScannerClient.listSidecarTypes({
          pageToken,
        });
        types.push(...page.sidecarTypes);
        pageToken = page.nextPageToken;
      } while (pageToken !== "");
      return types;
    },
  });
}

// ListSonarrInstances is paginated (AIP-158) with a server-side page cap, so
// the settings screen — which shows every instance at once — drains the
// pages rather than assuming one call returns them all. The collection is
// operator-bounded, so this is a handful of entries in one page in practice.
export function useSonarrInstances() {
  return useQuery({
    queryKey: queryKeys.sonarr,
    queryFn: async () => {
      const instances = [];
      let pageToken = "";
      do {
        const page = await sonarrInterfaceClient.listSonarrInstances({
          pageToken,
        });
        instances.push(...page.sonarrInstances);
        pageToken = page.nextPageToken;
      } while (pageToken !== "");
      return instances;
    },
  });
}

// The bus snapshot arrives over the socket rather than by refetching: the
// stream writes each frame straight into this query's cache entry. The
// queryFn still runs once, so the page has something to paint before the
// stream is up and something to fall back on if it never connects.
// Agents stream over the socket for the same reason the bus snapshot does: the
// telemetry is live and the presence half changes on its own, with no user
// action to hang a refetch off. The queryFn covers the first paint. Both the
// stream frame and the refetch yield the generated AgentView directly, so
// they land in the same cache entry with no shape mismatch.

// One singleton per server-streaming RPC, refcounted across every component
// watching it — see streams.ts. Registered so resetStreams() (called on
// sign-out from AuthContext.clearSession) can close all three at once.
const agentsPresenceStream = registerStream(
  new Stream((signal) =>
    mapAsync(
      agentClient.streamPresence({}, { signal }),
      (response) => response.agents,
    ),
  ),
);

export function useAgentsPresenceStreamStatus() {
  return useStreamStatus(agentsPresenceStream);
}

export function useAgents() {
  useStream(agentsPresenceStream, queryKeys.agents);

  return useQuery({
    queryKey: queryKeys.agents,
    queryFn: async () => (await agentClient.list({})).agents,
    staleTime: Infinity,
  });
}

// Agents are upserted by slug, like every other config collection here.
export function useUpsertAgent() {
  return useConfigMutation<
    MessageInitShape<typeof AgentConfigSchema>,
    ConnectAcceptedResponse
  >(
    (agent) => agentClient.upsert({ agent }),
    [queryKeys.config, queryKeys.agents],
  );
}

export function useDeleteAgent() {
  return useConfigMutation<string, ConnectAcceptedResponse>(
    (slug) => agentClient.delete({ slug }),
    [queryKeys.config, queryKeys.agents],
  );
}

// StatsService.Get/Stream carry a typed BusSnapshot. The server samples Redis
// on its own cadence and fans the snapshot out; the stream only ever yields
// frames that carry one (a frame without a snapshot is skipped rather than
// written to the cache as undefined, which react-query rejects), and the
// first-paint Get coalesces an absent snapshot to null for the same reason.
const busSnapshotStream = registerStream(
  new Stream(async function* (signal) {
    for await (const response of statsClient.stream({}, { signal })) {
      if (response.snapshot) yield response.snapshot;
    }
  }),
);

export function useBusSnapshotStreamStatus() {
  return useStreamStatus(busSnapshotStream);
}

export function useBusSnapshot() {
  useStream(busSnapshotStream, queryKeys.busSnapshot);

  return useQuery({
    queryKey: queryKeys.busSnapshot,
    queryFn: async () => (await statsClient.get({})).snapshot ?? null,
    staleTime: Infinity,
  });
}

// StatsService.Purge is the one write on the service: it clears a jammed
// durable stream (one by name, or every discovered one) server-side — an
// approximate trim plus a consumer-group fast-forward. There is no
// system_config_update event behind it, so like the workflow mutations it
// gets a plain useMutation rather than useConfigMutation. On success the bus
// snapshot is invalidated so the drained depth shows on the next frame
// without waiting for the sampler's own cadence.
export type PurgeStreamsTarget = { stream: string } | { all: true };

export function usePurgeStreams() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (target: PurgeStreamsTarget) =>
      statsClient.purge(
        "all" in target ? { all: true } : { stream: target.stream },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.busSnapshot });
    },
  });
}

export function useLoggingConfig() {
  return useQuery({
    queryKey: queryKeys.logging,
    queryFn: async () => (await loggingClient.getLoggingConfig({})).config,
  });
}

export function useEventBusConfig() {
  return useQuery({
    queryKey: queryKeys.eventBus,
    queryFn: async () => (await eventBusClient.getEventBusConfig({})).config,
  });
}

// LoggingService.GetTail/StreamTail carry typed LogRecord messages; the
// stream frame and the first-paint read land in the same cache entry.
const logTailStream = registerStream(
  new Stream((signal) =>
    mapAsync(
      loggingClient.streamTail({}, { signal }),
      (response) => response.records,
    ),
  ),
);

export function useLogTailStreamStatus() {
  return useStreamStatus(logTailStream);
}

// The live tail streams continuously; the queryFn covers first paint and a
// down stream.
export function useLogTail() {
  useStream(logTailStream, queryKeys.logTail);

  return useQuery({
    queryKey: queryKeys.logTail,
    queryFn: async () => (await loggingClient.getTail({})).records,
    staleTime: Infinity,
  });
}

// A dedicated sub-resource rather than a full AgentConfig upsert: setting a
// level should never risk touching an agent's mappings, and it works even for
// an agent that isn't configured with any yet (the server creates a bare
// entry) — see SetAgentLogLevel's doc comment on the Go side.
export function useSetAgentLogLevel() {
  return useConfigMutation<
    { slug: string; log_level: string },
    ConnectAcceptedResponse
  >(
    ({ slug, log_level }) =>
      agentClient.setLogLevel({ slug, logLevel: log_level }),
    [queryKeys.config, queryKeys.agents],
  );
}

/*
 * Writes. Each invalidates every query that could show the change: the config
 * document overlaps the scoped endpoints, so a scan directory edit has to
 * refresh both its own list and the whole-config read.
 */

// TResult defaults to the generated metarr.v1.AcceptedResponse message every
// config mutation returns; a still-migrating domain's mutationFn can name its
// own type instead. Nothing downstream reads these fields today — onSuccess
// only triggers invalidation.
function useConfigMutation<TVariables, TResult = ConnectAcceptedResponse>(
  mutationFn: (variables: TVariables) => Promise<TResult>,
  keysToInvalidate: readonly (readonly unknown[])[],
): UseMutationResult<TResult, Error, TVariables> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSuccess: () => {
      keysToInvalidate.forEach((key) => {
        void queryClient.invalidateQueries({ queryKey: key });
      });
    },
  });
}

export function useUpdateAdmin() {
  return useConfigMutation<
    MessageInitShape<typeof ConfigServiceUpdateAdminRequestSchema>,
    ConnectAcceptedResponse
  >((body) => configClient.updateAdmin(body), [queryKeys.config]);
}

export function useUpsertApiKey() {
  return useConfigMutation<
    MessageInitShape<typeof ConfigServiceUpsertApiKeyRequestSchema>,
    ConnectAcceptedResponse
  >((body) => configClient.upsertApiKey(body), [queryKeys.config]);
}

export function useDeleteApiKey() {
  return useConfigMutation<
    MessageInitShape<typeof ConfigServiceDeleteApiKeyRequestSchema>,
    ConnectAcceptedResponse
  >((body) => configClient.deleteApiKey(body), [queryKeys.config]);
}

// The keys of a config patch are camelCase message-field names; a protobuf
// FieldMask carries them lower_snake_case. Each scalar-section update sends
// only the fields the operator changed, so the patch's own keys are the mask
// (metarr.v1 AIP config CRUD, docs/adr/0010). The camelCase→snake_case rule is
// exact for these flat scalar sections; a later section with nested/dotted
// paths must pass its mask paths explicitly rather than lean on this. The `$`
// filter drops protobuf-es marker keys ($typeName) if a whole message is ever
// passed instead of a plain patch.
function updateMaskFor(patch: Record<string, unknown>): { paths: string[] } {
  return {
    paths: Object.keys(patch)
      .filter((key) => !key.startsWith("$"))
      .map((key) => key.replace(/[A-Z]/g, (c) => `_${c.toLowerCase()}`)),
  };
}

// A scalar-section partial update: just the changed fields (patch); the
// update_mask is derived from their keys.
type ScalarSectionUpdate<S extends DescMessage> = {
  patch: MessageInitShape<S>;
};

// The synchronous AIP write path shared by the scalar config sections (event
// bus, logging — docs/adr/0002). UpdateX merges the masked fields onto the
// section under the config store's lock and returns the *stored* section, so
// the response is authoritative: it is written straight into the section's
// query cache, no refetch and no queued→confirmed poll. The read-only
// aggregate GetConfig — a sibling query, ["config"], not a prefix of the
// section key — still overlaps the section's data, so it alone is invalidated,
// exact:true so the fuzzy match does not sweep the section read back in.
function useScalarSectionUpdate<S extends DescMessage, R>(
  sectionKey: readonly unknown[],
  write: (patch: MessageInitShape<S>) => Promise<R>,
): UseMutationResult<R, Error, ScalarSectionUpdate<S>> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ patch }: ScalarSectionUpdate<S>) => write(patch),
    onSuccess: (stored) => {
      queryClient.setQueryData(sectionKey, stored);
      void queryClient.invalidateQueries({
        queryKey: queryKeys.config,
        exact: true,
      });
    },
  });
}

// AIP-134 partial update: LoggingService.UpdateLoggingConfig merges the masked
// fields onto cfg.Logging under the config-store lock and returns it.
export function useUpdateLoggingConfig() {
  return useScalarSectionUpdate<
    typeof LoggingConfigSchema,
    ConnectLoggingConfig
  >(queryKeys.logging, (patch) =>
    loggingClient.updateLoggingConfig({
      config: patch,
      updateMask: updateMaskFor(patch),
    }),
  );
}

// AIP-134 partial update, same shape as logging: EventBusConfig is a flat
// block of scalars, so the patch's keys map one-for-one to the update_mask
// paths. The server merges them onto cfg.EventBus as a scoped mutation.
export function useUpdateEventBusConfig() {
  return useScalarSectionUpdate<
    typeof EventBusConfigSchema,
    ConnectEventBusConfig
  >(queryKeys.eventBus, (patch) =>
    eventBusClient.updateEventBusConfig({
      config: patch,
      updateMask: updateMaskFor(patch),
    }),
  );
}

// DirectoryScannerService is on AIP standard methods (docs/adr/0010): the
// scalar section's UpdateDirectoryScannerConfig and every sub-collection
// write is synchronous and returns the stored resource. Like the Sonarr
// hooks below, each write splices its own response into the section cache
// rather than refetching, and invalidates the sibling aggregate GetConfig
// read (["config"], exact, so the fuzzy match cannot sweep the section reads
// back in).

// AIP-134 partial update of the scalar section: the mask names only
// parallel_count and the response is the stored DirectoryScannerConfig,
// written straight into the section cache — no confirmation poll.
export function useUpdateDirectoryScanner() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ parallelCount }: { parallelCount: number }) =>
      directoryScannerClient.updateDirectoryScannerConfig({
        config: { parallelCount },
        updateMask: { paths: ["parallel_count"] },
      }),
    onSuccess: (stored: ConnectDirectoryScannerConfig) => {
      queryClient.setQueryData(queryKeys.directoryScanner, stored);
      void queryClient.invalidateQueries({
        queryKey: queryKeys.config,
        exact: true,
      });
    },
  });
}

function patchScanDirectoryListCache(
  queryClient: QueryClient,
  update: (current: ConnectScanDirectory[]) => ConnectScanDirectory[],
) {
  queryClient.setQueryData<ConnectScanDirectory[]>(
    queryKeys.scanDirectories,
    (current = []) => update(current),
  );
  void queryClient.invalidateQueries({
    queryKey: queryKeys.config,
    exact: true,
  });
}

function useScanDirectoryCollectionWrite<TVariables>(
  write: (variables: TVariables) => Promise<ConnectScanDirectory>,
): UseMutationResult<ConnectScanDirectory, Error, TVariables> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: write,
    onSuccess: (stored) => {
      patchScanDirectoryListCache(queryClient, (current) => {
        const index = current.findIndex(
          (entry) => entry.scannerSlug === stored.scannerSlug,
        );
        if (index === -1) return [...current, stored];
        const next = current.slice();
        next[index] = stored;
        return next;
      });
    },
  });
}

export function useCreateScanDirectory() {
  return useScanDirectoryCollectionWrite<
    MessageInitShape<typeof ScanDirectorySchema>
  >((directory) =>
    directoryScannerClient.createScanDirectory({
      scanDirectoryId: directory.scannerSlug,
      scanDirectory: directory,
    }),
  );
}

// The scan-directory editor sends the whole resource back, so the mask names
// every writable field — scanner_slug is the addressing key, set from the
// resource and never masked.
const scanDirectoryUpdateMask = { paths: ["scan_type", "directory"] };

export function useUpdateScanDirectory() {
  return useScanDirectoryCollectionWrite<
    MessageInitShape<typeof ScanDirectorySchema>
  >((directory) =>
    directoryScannerClient.updateScanDirectory({
      scanDirectory: directory,
      updateMask: scanDirectoryUpdateMask,
    }),
  );
}

export function useDeleteScanDirectory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (slug: string) =>
      directoryScannerClient.deleteScanDirectory({ slug }),
    onSuccess: (_result, slug) => {
      patchScanDirectoryListCache(queryClient, (current) =>
        current.filter((entry) => entry.scannerSlug !== slug),
      );
    },
  });
}

function patchSidecarTypeListCache(
  queryClient: QueryClient,
  next:
    | ConnectSidecarType[]
    | ((current: ConnectSidecarType[]) => ConnectSidecarType[]),
) {
  queryClient.setQueryData<ConnectSidecarType[]>(
    queryKeys.sidecarTypes,
    (current = []) => (typeof next === "function" ? next(current) : next),
  );
  void queryClient.invalidateQueries({
    queryKey: queryKeys.config,
    exact: true,
  });
}

function useSidecarTypeCollectionWrite<TVariables>(
  write: (variables: TVariables) => Promise<ConnectSidecarType>,
): UseMutationResult<ConnectSidecarType, Error, TVariables> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: write,
    onSuccess: (stored) => {
      patchSidecarTypeListCache(queryClient, (current) => {
        const index = current.findIndex((entry) => entry.id === stored.id);
        if (index === -1) return [...current, stored];
        const next = current.slice();
        next[index] = stored;
        return next;
      });
    },
  });
}

export function useCreateSidecarType() {
  return useSidecarTypeCollectionWrite<
    MessageInitShape<typeof SidecarTypeDefinitionSchema>
  >((sidecarType) => directoryScannerClient.createSidecarType({ sidecarType }));
}

// The editor sends the whole resource back; id is the addressing key and
// order belongs to ReorderSidecarTypes, so neither is masked.
const sidecarTypeUpdateMask = {
  paths: ["type", "category", "patterns", "extensions"],
};

export function useUpdateSidecarType() {
  return useSidecarTypeCollectionWrite<
    MessageInitShape<typeof SidecarTypeDefinitionSchema>
  >((sidecarType) =>
    directoryScannerClient.updateSidecarType({
      sidecarType,
      updateMask: sidecarTypeUpdateMask,
    }),
  );
}

export function useDeleteSidecarType() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      directoryScannerClient.deleteSidecarType({ id }),
    onSuccess: (_result, id) => {
      patchSidecarTypeListCache(queryClient, (current) =>
        current.filter((entry) => entry.id !== id),
      );
    },
  });
}

// Ordering covers the whole table in one call — it is the only place an entry
// can be enabled or disabled, since order zero is the disabled sentinel.
// Reorder and Reset are custom methods (AIP-136) that return the updated
// list, written straight into the section cache.
export function useReorderSidecarTypes() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (orders: Record<string, number>) =>
      directoryScannerClient.reorderSidecarTypes({ orders }),
    onSuccess: (result) => {
      patchSidecarTypeListCache(queryClient, result.sidecarTypes);
    },
  });
}

export function useResetSidecarTypes() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => directoryScannerClient.resetSidecarTypes({}),
    onSuccess: (result) => {
      patchSidecarTypeListCache(queryClient, result.sidecarTypes);
    },
  });
}

// SonarrInterfaceService is a collection on AIP standard methods
// (docs/adr/0010): Create / Update return the *stored* instance, Delete
// returns empty, and every write is synchronous. Rather than refetch the
// whole list, each write keeps the ["config","interfaces","sonarr"] cache
// current from its own response through patchSonarrListCache below.

// patchSonarrListCache applies update to the cached instance list and
// invalidates the sibling aggregate GetConfig read (["config"], exact, so
// the fuzzy match cannot sweep the list read back in). The one place every
// Sonarr write spells its cache contract.
function patchSonarrListCache(
  queryClient: QueryClient,
  update: (current: ConnectSonarrInstance[]) => ConnectSonarrInstance[],
) {
  queryClient.setQueryData<ConnectSonarrInstance[]>(
    queryKeys.sonarr,
    (current = []) => update(current),
  );
  void queryClient.invalidateQueries({
    queryKey: queryKeys.config,
    exact: true,
  });
}

function useSonarrCollectionWrite<TVariables>(
  write: (variables: TVariables) => Promise<ConnectSonarrInstance>,
): UseMutationResult<ConnectSonarrInstance, Error, TVariables> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: write,
    onSuccess: (stored) => {
      patchSonarrListCache(queryClient, (current) => {
        const index = current.findIndex(
          (entry) => entry.instanceSlug === stored.instanceSlug,
        );
        if (index === -1) return [...current, stored];
        const next = current.slice();
        next[index] = stored;
        return next;
      });
    },
  });
}

export function useCreateSonarrInstance() {
  return useSonarrCollectionWrite<
    MessageInitShape<typeof SonarrInstanceSchema>
  >((instance) =>
    sonarrInterfaceClient.createSonarrInstance({
      sonarrInstanceId: instance.instanceSlug,
      sonarrInstance: instance,
    }),
  );
}

// The Sonarr screen edits an instance by sending the whole resource back, so
// the update_mask names every writable field — the slug is the addressing
// key, set from the request and never the mask. A future partial editor
// would pass its own narrower mask instead. Keep this list in step with the
// writable fields of SonarrInstance in
// proto/metarr/v1/sonarr_interfaces.proto; queries.test.ts pins the exact
// paths.
const sonarrInstanceUpdateMask = {
  paths: [
    "instance_name",
    "sonarr_url",
    "sonarr_api_key",
    "root_dir_map",
    "storage",
  ],
};

export function useUpdateSonarrInstance() {
  return useSonarrCollectionWrite<
    MessageInitShape<typeof SonarrInstanceSchema>
  >((instance) =>
    sonarrInterfaceClient.updateSonarrInstance({
      sonarrInstance: instance,
      updateMask: sonarrInstanceUpdateMask,
    }),
  );
}

export function useDeleteSonarrInstance() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (slug: string) =>
      sonarrInterfaceClient.deleteSonarrInstance({ slug }),
    onSuccess: (_result, slug) => {
      patchSonarrListCache(queryClient, (current) =>
        current.filter((entry) => entry.instanceSlug !== slug),
      );
    },
  });
}

/*
 * Workflows. Unlike everything above, these are a direct, synchronous Mongo
 * read/write with no config_update event behind them — see the Go handler's
 * doc comment on UpsertWorkflow — so they get their own mutation hook rather
 * than useConfigMutation, whose return type is hardwired to AcceptedResponse.
 */

export function useWorkflow(id: string) {
  return useQuery({
    queryKey: queryKeys.workflow(id),
    queryFn: async () => {
      const { workflow } = await workflowClient.get({ id });
      return workflow;
    },
    enabled: id !== "",
  });
}

export function useWorkflowVersions(id: string) {
  return useQuery({
    queryKey: queryKeys.workflowVersions(id),
    queryFn: async () => (await workflowClient.listVersions({ id })).versions,
    enabled: id !== "",
  });
}

export function useWorkflowVersion(id: string, version: number | null) {
  return useQuery({
    queryKey: [...queryKeys.workflow(id), "v", version],
    queryFn: async () => {
      const { workflow } = await workflowClient.getVersion({
        id,
        version: version ?? 0,
      });
      return workflow;
    },
    enabled: id !== "" && version != null,
  });
}

// Infinite-scroll list, paginated by the opaque cursor List returns.
export function useWorkflowList() {
  return useInfiniteQuery({
    queryKey: queryKeys.workflows,
    queryFn: async ({ pageParam }: { pageParam: string | undefined }) => {
      const response = await workflowClient.list({
        limit: 20,
        cursor: pageParam ?? "",
      });
      return {
        workflows: response.workflows,
        nextCursor: response.nextCursor,
        hasMore: response.hasMore,
      };
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) =>
      lastPage.hasMore ? lastPage.nextCursor : undefined,
  });
}

// The node/socket/transform catalog the palette, the node renderers, and
// isValidConnection all read from. It arrives as a typed WorkflowCatalog
// message now (docs/adr/0005), not an opaque JSON blob. staleTime: Infinity
// like useAgents/useConfig: it only changes on a server redeploy, never
// mid-session, so there's no socket topic behind it — just a plain
// fetch-once query.
export function useWorkflowCatalog() {
  return useQuery({
    queryKey: queryKeys.workflowCatalog,
    queryFn: async () => {
      const { catalog } = await workflowCatalogClient.get({});
      return catalog;
    },
    staleTime: Infinity,
  });
}

// The upsert body is the generated request's init shape — no hand-maintained
// copy of its fields to keep in step (docs/adr/0005).
export type SaveWorkflowInput = MessageInitShape<
  typeof WorkflowServiceUpsertRequestSchema
>;

export function useSaveWorkflow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: SaveWorkflowInput) => {
      const { workflow } = await workflowClient.upsert(body);
      if (!workflow) throw new Error("save did not return a workflow");
      return workflow;
    },
    onSuccess: (saved) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.workflows });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.workflow(saved.documentId),
      });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.workflowVersions(saved.documentId),
      });
    },
  });
}
