import { createClient } from '@connectrpc/connect'

import { AgentService } from '../gen/metarr/v1/agents_pb'
import { AuthService } from '../gen/metarr/v1/auth_pb'
import { ConfigService } from '../gen/metarr/v1/config_pb'
import { DirectoryScannerService } from '../gen/metarr/v1/directory_scanner_pb'
import { EventBusService } from '../gen/metarr/v1/event_bus_pb'
import { LoggingService } from '../gen/metarr/v1/logging_pb'
import { SonarrInterfaceService } from '../gen/metarr/v1/sonarr_interfaces_pb'
import { StatsService } from '../gen/metarr/v1/stats_pb'
import { WorkflowCatalogService } from '../gen/metarr/v1/workflow_catalog_pb'
import { WorkflowService } from '../gen/metarr/v1/workflows_pb'
import { transport } from './transport'

// One generated client per service, all sharing the one transport (and so
// the one auth interceptor) from transport.ts. Added here as each domain
// migrates off REST — see the migration plan's per-step ordering.
export const sonarrInterfaceClient = createClient(
  SonarrInterfaceService,
  transport,
)
export const authClient = createClient(AuthService, transport)
export const configClient = createClient(ConfigService, transport)
export const agentClient = createClient(AgentService, transport)
export const directoryScannerClient = createClient(
  DirectoryScannerService,
  transport,
)
export const loggingClient = createClient(LoggingService, transport)
export const eventBusClient = createClient(EventBusService, transport)
export const workflowCatalogClient = createClient(
  WorkflowCatalogService,
  transport,
)
export const workflowClient = createClient(WorkflowService, transport)
export const statsClient = createClient(StatsService, transport)
