import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
  type UseMutationResult,
} from "@tanstack/react-query";

import {
  adminClient,
  agentClient,
  apiKeyClient,
  authClient,
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
import type { EventBusConfig as ConnectEventBusConfig } from "../gen/metarr/v1/event_bus_pb";
import type { LoggingConfig as ConnectLoggingConfig } from "../gen/metarr/v1/logging_pb";
import {
  SonarrInstanceSchema,
  type SonarrInstance as ConnectSonarrInstance,
} from "../gen/metarr/v1/sonarr_interfaces_pb";
import {
  AuthenticationScheme,
  type AdminUser as ConnectAdminUser,
} from "../gen/metarr/v1/admin_pb";
import type { Config as ConnectConfig } from "../gen/metarr/v1/config_pb";
import {
  AccessLevel,
  type APIKeyEntry as ConnectAPIKeyEntry,
} from "../gen/metarr/v1/api_keys_pb";
import { AgentSchema } from "../gen/metarr/v1/agents_pb";
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
  // Outside the config tree: the pre-login scheme probe is unauthenticated
  // and drives the app's render gate, not a settings screen.
  authScheme: ["auth-scheme"] as const,
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
 * Every config write is synchronous now (docs/adr/0002): the RPC returns the
 * stored resource and its hook splices that into the query cache, so a
 * settled mutation means the read is already fresh — there is no
 * queued→confirmed poll (useSaveState no longer re-reads).
 */

export function useConfig() {
  return useQuery({
    queryKey: queryKeys.config,
    queryFn: async () => (await configClient.getConfig({})).config,
  });
}

// The active authentication scheme, read through the unauthenticated
// AuthService.GetAuthScheme probe (docs/adr/0012). App.tsx runs this before
// its first render-gate decision so a cold load never flashes the app shell
// on the way to the login screen. The server normalises the value, so it is
// never AuthenticationScheme.UNSPECIFIED. It only changes when an operator
// edits it on the Security page, and that mutation invalidates this key.
export function useAuthScheme() {
  return useQuery({
    queryKey: queryKeys.authScheme,
    queryFn: async () => (await authClient.getAuthScheme({})).scheme,
    staleTime: Infinity,
  });
}

export function useDirectoryScannerConfig() {
  return useQuery({
    queryKey: queryKeys.directoryScanner,
    queryFn: async () =>
      (await directoryScannerClient.getDirectoryScannerConfig({})).config,
  });
}

// Every config List RPC is the AIP-158 / 132 / 160 contract: page_size /
// page_token / order_by / filter (filter translation is still deferred
// server-side — a non-empty filter is Unimplemented). The settings screens
// show a whole operator-bounded collection at once, so these hooks drain the
// pages; order_by / filter / page_size are surfaced for callers that want
// them even though no screen passes any yet.
export type ListOptions = {
  orderBy?: string;
  filter?: string;
  pageSize?: number;
};

// listKey keeps the default (no-options) call keyed exactly as before, so the
// collection write hooks that splice into that cache entry still hit it.
function listKey(base: readonly unknown[], options: ListOptions) {
  return Object.keys(options).length > 0 ? [...base, options] : base;
}

async function drainPages<T>(
  fetchPage: (
    pageToken: string,
  ) => Promise<{ items: T[]; nextPageToken: string }>,
): Promise<T[]> {
  const all: T[] = [];
  let pageToken = "";
  do {
    const page = await fetchPage(pageToken);
    all.push(...page.items);
    pageToken = page.nextPageToken;
  } while (pageToken !== "");
  return all;
}

export function useScanDirectories(options: ListOptions = {}) {
  return useQuery({
    queryKey: listKey(queryKeys.scanDirectories, options),
    queryFn: () =>
      drainPages(async (pageToken) => {
        const page = await directoryScannerClient.listScanDirectories({
          pageToken,
          orderBy: options.orderBy,
          filter: options.filter,
          pageSize: options.pageSize,
        });
        return {
          items: page.scanDirectories,
          nextPageToken: page.nextPageToken,
        };
      }),
  });
}

export function useSidecarTypes(options: ListOptions = {}) {
  return useQuery({
    queryKey: listKey(queryKeys.sidecarTypes, options),
    queryFn: () =>
      drainPages(async (pageToken) => {
        const page = await directoryScannerClient.listSidecarTypes({
          pageToken,
          orderBy: options.orderBy,
          filter: options.filter,
          pageSize: options.pageSize,
        });
        return { items: page.sidecarTypes, nextPageToken: page.nextPageToken };
      }),
  });
}

export function useSonarrInstances(options: ListOptions = {}) {
  return useQuery({
    queryKey: listKey(queryKeys.sonarr, options),
    queryFn: () =>
      drainPages(async (pageToken) => {
        const page = await sonarrInterfaceClient.listSonarrInstances({
          pageToken,
          orderBy: options.orderBy,
          filter: options.filter,
          pageSize: options.pageSize,
        });
        return {
          items: page.sonarrInstances,
          nextPageToken: page.nextPageToken,
        };
      }),
  });
}

// The bus snapshot arrives over the socket rather than by refetching: the
// stream writes each frame straight into this query's cache entry. The
// queryFn still runs once, so the page has something to paint before the
// stream is up and something to fall back on if it never connects.
// Agents stream over the socket for the same reason the bus snapshot does: the
// telemetry is live and the presence half changes on its own, with no user
// action to hang a refetch off. The queryFn covers the first paint. Both the
// stream frame and the refetch yield the generated Agent directly, so they
// land in the same cache entry with no shape mismatch.

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

// ListAgents is paginated (AIP-158); the screen shows every agent at once, so
// it drains the pages. This cache entry is also written by StreamPresence
// (the merged live view), so unlike the other List hooks it is not keyed by
// ListOptions — order_by / filter on a socket-merged list would need a
// separate non-streamed hook.
export function useAgents() {
  useStream(agentsPresenceStream, queryKeys.agents);

  return useQuery({
    queryKey: queryKeys.agents,
    queryFn: () =>
      drainPages(async (pageToken) => {
        const page = await agentClient.listAgents({ pageToken });
        return { items: page.agents, nextPageToken: page.nextPageToken };
      }),
    staleTime: Infinity,
  });
}

// AgentService is a slug-addressed collection on AIP standard methods
// (docs/adr/0010): Create / Update return the stored Agent, Delete returns
// empty, every write is synchronous. The agents cache (["stats","agents"]) is
// socket-fed by StreamPresence and carries the merged presence view, so a
// write invalidates it — the refetch re-merges live presence — rather than
// splicing a presence-less response in. The sibling aggregate GetConfig read
// is invalidated exact.
function useAgentCollectionWrite<TVariables>(
  write: (variables: TVariables) => Promise<unknown>,
): UseMutationResult<unknown, Error, TVariables> {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: write,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.agents });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.config,
        exact: true,
      });
    },
  });
}

export function useCreateAgent() {
  return useAgentCollectionWrite<MessageInitShape<typeof AgentSchema>>(
    (agent) => agentClient.createAgent({ agentId: agent.slug, agent }),
  );
}

// The Agents screen edits display name and library mappings; the log level is
// the Logging screen's job (SetLogLevel), so it is not in this mask even
// though it is a writable field. slug is the addressing key and the presence
// fields are output-only, so neither is masked either.
const agentUpdateMask = { paths: ["display_name", "mappings"] };

export function useUpdateAgent() {
  return useAgentCollectionWrite<MessageInitShape<typeof AgentSchema>>(
    (agent) => agentClient.updateAgent({ agent, updateMask: agentUpdateMask }),
  );
}

export function useDeleteAgent() {
  return useAgentCollectionWrite<string>((slug) =>
    agentClient.deleteAgent({ slug }),
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
// approximate trim plus a consumer-group fast-forward. On success the bus
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

// A custom method rather than a full Agent update: setting a level should
// never risk touching an agent's mappings, and it works even for an agent
// that isn't configured with any yet (the server creates a bare entry) — see
// SetLogLevel's doc comment on the Go side. It returns the stored Agent on
// the same socket-fed cache as the other agent writes, so it reconciles the
// same way (invalidate, re-merge live presence).
export function useSetAgentLogLevel() {
  return useAgentCollectionWrite<{ slug: string; log_level: string }>(
    ({ slug, log_level }) =>
      agentClient.setLogLevel({ slug, logLevel: log_level }),
  );
}

/*
 * Writes. Config writes are synchronous (docs/adr/0002): each RPC returns the
 * stored resource, and the hook splices that response into the query cache —
 * a scoped list, or the aggregate ["config"] document — rather than
 * invalidating and refetching. The agent writes are the exception: their
 * cache is socket-fed by StreamPresence, so they invalidate and let the
 * refetch re-merge live presence.
 */

// AdminService and ApiKeyService are synchronous AIP writes (docs/adr/0002):
// each returns the stored resource, and there is no dedicated query for it —
// the Security screen paints from the aggregate GetConfig read (["config"]).
// So rather than invalidate and refetch the whole document, each write
// splices its own response into that cached Config through patchConfigCache.

const apiKeyGroups = ["admin", "user", "webhook", "readOnly"] as const;
type ApiKeyGroup = (typeof apiKeyGroups)[number];

// The AccessLevel enum a request carries, mapped to the field it addresses on
// the stored APIKeysConfig. UNSPECIFIED has no group — the server rejects it,
// so the cache splice just skips.
const apiKeyGroupOf: Partial<Record<AccessLevel, ApiKeyGroup>> = {
  [AccessLevel.ADMIN]: "admin",
  [AccessLevel.USER]: "user",
  [AccessLevel.WEBHOOK]: "webhook",
  [AccessLevel.READ_ONLY]: "readOnly",
};

function patchConfigCache(
  queryClient: QueryClient,
  update: (config: ConnectConfig) => ConnectConfig,
) {
  const existing = queryClient.getQueryData<ConnectConfig>(queryKeys.config);
  if (!existing) {
    // The aggregate read the Security screen paints from is not populated —
    // nothing to splice into, so fall back to a refetch.
    void queryClient.invalidateQueries({
      queryKey: queryKeys.config,
      exact: true,
    });
    return;
  }
  queryClient.setQueryData<ConnectConfig>(queryKeys.config, update(existing));
}

// patchApiKeyGroup rewrites one APIKeysConfig group's entries in the cached
// aggregate Config, preserving the branded message shape.
function patchApiKeyGroup(
  queryClient: QueryClient,
  group: ApiKeyGroup,
  update: (entries: ConnectAPIKeyEntry[]) => ConnectAPIKeyEntry[],
) {
  patchConfigCache(queryClient, (config) => {
    const existing = config.apiKeys ?? {
      $typeName: "metarr.v1.APIKeysConfig" as const,
      admin: [],
      user: [],
      webhook: [],
      readOnly: [],
    };
    return {
      ...config,
      apiKeys: { ...existing, [group]: update(existing[group] ?? []) },
    };
  });
}

// patchApiKeyById is patchApiKeyGroup for the id-addressed writes: id names a
// key without an access level, so the splice first finds which group holds
// it. An id no cached group holds is a no-op.
function patchApiKeyById(
  queryClient: QueryClient,
  id: string,
  update: (entries: ConnectAPIKeyEntry[]) => ConnectAPIKeyEntry[],
) {
  const config = queryClient.getQueryData<ConnectConfig>(queryKeys.config);
  const group = apiKeyGroups.find((g) =>
    (config?.apiKeys?.[g] ?? []).some((entry) => entry.id === id),
  );
  if (group) patchApiKeyGroup(queryClient, group, update);
}

// AdminService.UpdateAdminUser is an AIP-134 partial update: update_mask
// names the identity fields the operator changed (username / email) and a
// new password rides new_password, never the mask (docs/adr/0010). The
// caller passes only what it is changing; the hook derives the mask. The
// response is the stored account with the credential blanked.
type AdminPatch = {
  username?: string;
  email?: string;
  password?: string;
  authenticationScheme?: AuthenticationScheme;
};

export function useUpdateAdmin() {
  const queryClient = useQueryClient();
  return useMutation<ConnectAdminUser, Error, AdminPatch>({
    mutationFn: ({ username, email, password, authenticationScheme }) => {
      const admin: {
        username?: string;
        email?: string;
        authenticationScheme?: AuthenticationScheme;
      } = {};
      if (username !== undefined) admin.username = username;
      if (email !== undefined) admin.email = email;
      if (authenticationScheme !== undefined)
        admin.authenticationScheme = authenticationScheme;
      return adminClient.updateAdminUser({
        admin,
        updateMask: updateMaskFor(admin),
        newPassword: password ?? "",
      });
    },
    onSuccess: (stored, patch) => {
      patchConfigCache(queryClient, (config) => ({ ...config, admin: stored }));
      // The render gate reads the scheme through its own unauthenticated
      // probe (useAuthScheme); keep that cache coherent when the operator
      // changes the scheme here.
      if (patch.authenticationScheme !== undefined) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.authScheme });
      }
    },
  });
}

// ApiKeyService is a minted-id collection scoped by the AccessLevel enum
// (docs/adr/0010). Create takes no id — the server mints one and returns the
// entry; Update is a FieldMask partial update matched by id; Delete is
// id-only. Each write reconciles the addressed group in the cached Config
// from its own response.
export function useCreateApiKey() {
  const queryClient = useQueryClient();
  return useMutation<
    ConnectAPIKeyEntry,
    Error,
    { accessLevel: AccessLevel; name: string }
  >({
    mutationFn: ({ accessLevel, name }) =>
      apiKeyClient.createApiKey({ accessLevel, apiKey: { name } }),
    onSuccess: (stored, { accessLevel }) => {
      const group = apiKeyGroupOf[accessLevel];
      if (group) {
        patchApiKeyGroup(queryClient, group, (entries) => [...entries, stored]);
      }
    },
  });
}

export function useUpdateApiKey() {
  const queryClient = useQueryClient();
  return useMutation<
    ConnectAPIKeyEntry,
    Error,
    { id: string; name?: string; apiKey?: string }
  >({
    mutationFn: ({ id, name, apiKey }) => {
      const fields: { name?: string; apiKey?: string } = {};
      if (name !== undefined) fields.name = name;
      if (apiKey !== undefined) fields.apiKey = apiKey;
      return apiKeyClient.updateApiKey({
        apiKey: { id, ...fields },
        updateMask: updateMaskFor(fields),
      });
    },
    onSuccess: (stored) => {
      patchApiKeyById(queryClient, stored.id, (entries) =>
        entries.map((entry) => (entry.id === stored.id ? stored : entry)),
      );
    },
  });
}

export function useDeleteApiKey() {
  // Delete-by-id, bare string like useDeleteSonarrInstance / useDeleteAgent.
  const queryClient = useQueryClient();
  return useMutation<unknown, Error, string>({
    mutationFn: (id) => apiKeyClient.deleteApiKey({ id }),
    onSuccess: (_result, id) => {
      patchApiKeyById(queryClient, id, (entries) =>
        entries.filter((entry) => entry.id !== id),
      );
    },
  });
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
 * Workflows. These are a direct Mongo read/write with no config document
 * overlap — see the Go handler's doc comment on UpsertWorkflow — so they
 * invalidate only the workflow keys.
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
