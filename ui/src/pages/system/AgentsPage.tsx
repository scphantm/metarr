import { useState } from 'react'
import { Alert, Badge, Progress, Space, Typography } from 'antd'

import { useAgents, useAgentsPresenceStreamStatus, useScanDirectories } from '../../api/queries'
import type { AgentTelemetry, AgentView } from '../../api/types'
import { Button, Card, EmptyState } from '../../components/Card'
import { PageError, PageLoading } from '../../components/PageState'
import { PageHeader } from '../../layout/AppShell'
import { AgentConfigureForm } from './AgentConfigureForm'
import './AgentsPage.css'

/*
 * System > Agents.
 *
 * An agent announces itself simply by connecting to Redis, so this screen shows
 * the union of two different things: agents someone has configured, and agents
 * that are currently there. Either can exist without the other, and the
 * difference matters — a configured agent that has gone quiet is a machine to go
 * and check on, while an unconfigured one that has appeared is a machine waiting
 * to be set up.
 */
export function AgentsPage() {
  const agents = useAgents()
  const directories = useScanDirectories()
  const socketStatus = useAgentsPresenceStreamStatus()

  const [configuring, setConfiguring] = useState<string | null>(null)

  if (agents.error && !agents.data) {
    return (
      <>
        <PageHeader title="Agents" />
        <PageError error={agents.error} />
      </>
    )
  }

  if (!agents.data) {
    return (
      <>
        <PageHeader title="Agents" />
        <PageLoading>Looking for agents…</PageLoading>
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Agents"
        description="Agents run next to your media and do every filesystem operation. They connect to Redis with nothing but a name; everything else is configured here."
        actions={<ConnectionIndicator status={socketStatus} />}
      />

      <div className="page-body">
        {agents.data.length === 0 ? (
          <EmptyState>
            No agents yet. Start a metarr-agent pointed at this Redis and it
            will appear here within a few seconds.
          </EmptyState>
        ) : null}

        {agents.data.map((agent) => (
          <AgentCard
            key={agent.slug}
            agent={agent}
            scanDirectories={directories.data ?? []}
            configuring={configuring === agent.slug}
            onConfigure={() => setConfiguring(agent.slug)}
            onClose={() => setConfiguring(null)}
          />
        ))}
      </div>
    </>
  )
}

function ConnectionIndicator({ status }: { status: string }) {
  const label =
    status === 'open' ? 'Live' : status === 'connecting' ? 'Connecting' : 'Stale'
  const badgeStatus =
    status === 'open' ? 'success' : status === 'connecting' ? 'processing' : 'warning'

  return <Badge status={badgeStatus} text={label} />
}

function AgentCard({
  agent,
  scanDirectories,
  configuring,
  onConfigure,
  onClose,
}: {
  agent: AgentView
  scanDirectories: { scanner_slug: string; directory: string }[]
  configuring: boolean
  onConfigure: () => void
  onClose: () => void
}) {
  const needsSetup = agent.online && !agent.configured

  return (
    <Card
      title={agent.display_name || agent.slug}
      description={agent.display_name ? agent.slug : undefined}
      actions={
        <Space align="center">
          <AgentStatus agent={agent} />
          {configuring ? null : (
            <Button variant={needsSetup ? 'primary' : 'default'} onClick={onConfigure}>
              {agent.configured ? 'Edit' : 'Configure this agent'}
            </Button>
          )}
        </Space>
      }
    >
      {needsSetup ? (
        <Alert
          type="info"
          showIcon
          className="agent-setup-notice"
          message="This agent has connected but has not been set up yet. Map the libraries it can reach to start using it."
        />
      ) : null}

      {agent.identity ? <AgentIdentityGrid agent={agent} /> : null}

      {agent.telemetry ? <TelemetryMeters telemetry={agent.telemetry} /> : null}

      {configuring ? (
        <AgentConfigureForm
          agent={agent}
          scanDirectories={scanDirectories}
          onDone={onClose}
        />
      ) : (
        <MappingList agent={agent} />
      )}
    </Card>
  )
}

// Status is a word first and a colour second: an operator scanning a column of
// cards reads the word, and colour alone would carry the whole meaning for
// nobody who cannot separate the hues.
function AgentStatus({ agent }: { agent: AgentView }) {
  if (!agent.online) {
    return <Badge status="default" text="Offline" />
  }
  if (!agent.configured) {
    return <Badge status="warning" text="Needs setup" />
  }
  return <Badge status="success" text="Online" />
}

function AgentIdentityGrid({ agent }: { agent: AgentView }) {
  const identity = agent.identity
  if (!identity) return null

  const facts: [string, string][] = [
    ['Host', identity.hostname || '—'],
    ['Address', identity.ip || '—'],
    ['Running as', `${identity.username || 'unknown'} (uid ${identity.uid})`],
    ['Platform', `${identity.os}/${identity.arch}`],
    ['Version', identity.version],
    ['Up since', new Date(identity.started).toLocaleString()],
  ]

  return (
    <div className="agent-identity-grid">
      {facts.map(([label, value]) => (
        <div key={label}>
          <div className="agent-identity-label">{label}</div>
          <div className="agent-identity-value" title={value}>
            {value}
          </div>
        </div>
      ))}
    </div>
  )
}

// CPU and memory are each one value against a known limit, so they are meters —
// a filled track against the same-hue background. A chart of two numbers would
// say less than the numbers do.
function TelemetryMeters({ telemetry }: { telemetry: AgentTelemetry }) {
  const memoryPercent = telemetry.memory_total_bytes
    ? (telemetry.memory_used_bytes / telemetry.memory_total_bytes) * 100
    : 0

  return (
    <Space direction="vertical" size={12} className="agent-telemetry-meters">
      <Meter
        label="CPU"
        percent={telemetry.cpu_percent}
        detail={`${telemetry.cpu_percent.toFixed(1)}%`}
      />
      <Meter
        label="Memory"
        percent={memoryPercent}
        detail={`${formatBytes(telemetry.memory_used_bytes)} of ${formatBytes(
          telemetry.memory_total_bytes,
        )}`}
      />
      {(telemetry.gpus ?? []).map((gpu, index) => (
        <Meter
          key={`${gpu.name}-${index}`}
          label={gpu.name || 'GPU'}
          percent={gpu.utilization_percent}
          detail={`${gpu.utilization_percent.toFixed(0)}% · ${formatBytes(
            gpu.memory_used_bytes,
          )} of ${formatBytes(gpu.memory_total_bytes)}`}
        />
      ))}
    </Space>
  )
}

function Meter({
  label,
  percent,
  detail,
}: {
  label: string
  percent: number
  detail: string
}) {
  const clamped = Math.max(0, Math.min(100, percent))

  return (
    <div>
      <div className="agent-meter-header">
        <Typography.Text style={{ fontSize: 12 }}>{label}</Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {detail}
        </Typography.Text>
      </div>
      <Progress percent={clamped} showInfo={false} size="small" aria-label={label} />
    </div>
  )
}

function MappingList({ agent }: { agent: AgentView }) {
  if (agent.mappings.length === 0) {
    return (
      <Typography.Text type="secondary" style={{ fontSize: 14 }}>
        No libraries mapped, so this agent has nothing to scan.
      </Typography.Text>
    )
  }

  return (
    <Space direction="vertical" size={4} style={{ width: '100%' }}>
      <Typography.Text type="secondary" className="agent-mapping-heading">
        Mapped libraries
      </Typography.Text>
      {agent.mappings.map((mapping) => (
        <div key={mapping.scanner_slug} className="agent-mapping-row">
          <span className="agent-mapping-slug">{mapping.scanner_slug}</span>
          <span className="agent-mapping-path">{mapping.server_path || '—'}</span>
          <Typography.Text type="secondary" aria-hidden="true">
            →
          </Typography.Text>
          <span className="agent-mapping-path is-local">{mapping.agent_path}</span>
        </div>
      ))}
    </Space>
  )
}

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const exponent = Math.min(
    units.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1024)),
  )
  const value = bytes / 1024 ** exponent
  return `${value.toFixed(value < 10 && exponent > 0 ? 1 : 0)} ${units[exponent]}`
}

export function AgentsSidebar() {
  return (
    <div className="saving-info-sidebar">
      <Alert
        type="info"
        message="How agents work"
        description={
          <>
            <p>
              An agent is configured locally with only two things: how to reach Redis, and its
              own name. Everything else — which libraries it can see and where they live on its
              machine — is published to it from here.
            </p>
            <p>
              It never connects to the database. Scan results travel back over the event bus and
              are stored under this server&apos;s paths, so the library reads the same however
              many agents produced it.
            </p>
          </>
        }
      />

      <Alert
        type="info"
        message="Mapping libraries"
        description={
          <>
            <p>
              A mapping says what this machine calls a library you have configured under
              Directory Scanner. Leave one blank when the agent cannot reach it — agents sit on
              different machines and are not expected to see everything.
            </p>
            <p>
              Each library belongs to one agent. Two agents scanning the same files would each
              overwrite the other&apos;s records.
            </p>
          </>
        }
      />
    </div>
  )
}
