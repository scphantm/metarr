import { useEffect, useState } from 'react'
import { Alert, Badge, Col, Row as AntRow, Table, Tag, Tooltip, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { timestampDate } from '@bufbuild/protobuf/wkt'

import { useBusSnapshot, useBusSnapshotStreamStatus } from '../../api/queries'
import type { StreamStatus } from '../../api/streams'
import type {
  BusChannelStat,
  BusGroupStat,
  BusServerInfo,
  BusStreamStat,
} from '../../gen/metarr/v1/stats_pb'
import { Card } from '../../components/Card'
import { PageError, PageLoading } from '../../components/PageState'
import { PageHeader } from '../../layout/AppShell'
import { Sparkline } from './Sparkline'
import './SystemDashboardPage.css'

/*
 * The system dashboard — the landing screen.
 *
 * A single server-side sampler polls Redis on a fixed cadence into one shared
 * snapshot and fans it out here; opening a second dashboard adds no Redis
 * load. The six server tiles and the durable-stream table render live off
 * that stream; the Pub/Sub channel table arrives in a later ticket and
 * renders empty until then.
 */

// A snapshot older than this many milliseconds is stale: the sampler ticks
// every ~2s, so four missed passes means the picture can no longer be
// trusted as live.
const STALE_AFTER_MS = 8_000

export function SystemDashboardPage() {
  const snapshot = useBusSnapshot()
  const streamStatus = useBusSnapshotStreamStatus()

  // Only a failure with nothing cached is fatal to the page. Once a snapshot
  // has arrived, a dropped stream keeps showing it and says so, because a
  // blank dashboard reads as "nothing is running" rather than "I can't see".
  if (snapshot.error && !snapshot.data) {
    return (
      <>
        <PageHeader title="System" />
        <PageError error={snapshot.error} />
      </>
    )
  }

  if (!snapshot.data) {
    return (
      <>
        <PageHeader title="System" />
        <PageLoading>Connecting to the event bus…</PageLoading>
      </>
    )
  }

  const data = snapshot.data

  return (
    <>
      <PageHeader
        title="System"
        description="Live statistics for the Redis instance behind the event bus."
        actions={
          <LivenessBadge status={streamStatus} lastFrameAt={snapshot.dataUpdatedAt} />
        }
      />

      <div className="page-body">
        {data.server ? <ServerTiles server={data.server} /> : null}

        <Card
          title="Event streams"
          description="Durable Redis Streams. Events sit on a stream until a consumer group acknowledges them, so depth and pending are real counts. Expand a row for its per-group figures."
        >
          <StreamsTable streams={data.streams} />
        </Card>

        <Card
          title="Pub/Sub channels"
          description="Redis Pub/Sub delivers to whoever is connected at that instant and keeps nothing, so these channels have subscribers but no depth to report."
        >
          <ChannelsTable channels={data.channels} />
        </Card>

        <Typography.Text type="secondary" className="system-dashboard-collected">
          Last collected{' '}
          {data.collectedAt
            ? timestampDate(data.collectedAt).toLocaleTimeString()
            : '—'}
        </Typography.Text>
      </div>
    </>
  )
}

type Liveness = 'live' | 'reconnecting' | 'stale'

// The connection state is worth showing plainly: a frozen dashboard and idle
// infrastructure look identical otherwise. A brief drop shows "Reconnecting"
// and clears itself once the stream re-establishes — the stream layer
// retries with backoff on its own.
//
// Freshness is measured against lastFrameAt — the client wall-clock time the
// last frame landed in the query cache (react-query's dataUpdatedAt) — not
// the server's collected_at, so a skewed browser clock can't pin a healthy
// stream to "Stale".
function LivenessBadge({
  status,
  lastFrameAt,
}: {
  status: StreamStatus
  lastFrameAt: number
}) {
  const now = useNow(2_000)

  const fresh = lastFrameAt > 0 && now - lastFrameAt < STALE_AFTER_MS

  let liveness: Liveness
  if (status === 'open' && fresh) {
    liveness = 'live'
  } else if (status === 'connecting') {
    liveness = 'reconnecting'
  } else {
    liveness = 'stale'
  }

  const label =
    liveness === 'live' ? 'Live' : liveness === 'reconnecting' ? 'Reconnecting' : 'Stale'
  const badgeStatus =
    liveness === 'live' ? 'success' : liveness === 'reconnecting' ? 'processing' : 'warning'

  return <Badge status={badgeStatus} text={label} />
}

// Re-render on an interval so a snapshot ageing past the stale threshold is
// noticed even while no new frame is arriving to trigger one.
function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs)
    return () => clearInterval(id)
  }, [intervalMs])
  return now
}

function ServerTiles({ server }: { server: BusServerInfo }) {
  const errored = (field: string) => server.fieldErrors[field]

  const tiles: {
    label: string
    value: string
    field?: string
    series?: bigint[]
  }[] = [
    { label: 'Version', value: server.version || '—', field: 'version' },
    {
      label: 'Uptime',
      value: formatDuration(Number(server.uptimeSeconds)),
      field: 'uptime_seconds',
    },
    {
      label: 'Connected clients',
      value: Number(server.connectedClients).toLocaleString(),
      field: 'connected_clients',
      series: server.connectedClientsSeries,
    },
    {
      label: 'Memory used',
      value: server.usedMemoryHuman || formatBytes(Number(server.usedMemory)),
      field: 'used_memory',
      series: server.usedMemorySeries,
    },
    {
      label: 'Ops / second',
      value: Number(server.opsPerSecond).toLocaleString(),
      field: 'ops_per_second',
      series: server.opsPerSecondSeries,
    },
    {
      label: 'Keys',
      value: Number(server.totalKeys).toLocaleString(),
      field: 'total_keys',
      series: server.totalKeysSeries,
    },
  ]

  return (
    <AntRow gutter={[12, 12]}>
      {tiles.map((tile) => {
        const error = tile.field ? errored(tile.field) : undefined
        return (
          <Col key={tile.label} xs={12} sm={8} lg={4}>
            <div className="system-dashboard-tile">
              <div className="system-dashboard-tile-value">{error ? '—' : tile.value}</div>
              <div className="system-dashboard-tile-label">{tile.label}</div>
              {error ? (
                <div className="system-dashboard-tile-error" title={error}>
                  unavailable
                </div>
              ) : tile.series ? (
                <Sparkline values={tile.series} title={`${tile.label} trend`} />
              ) : null}
            </div>
          </Col>
        )
      })}
    </AntRow>
  )
}

// MetricCell pairs a right-aligned numeric value with the inline sparkline of
// its rolling series. Sparkline itself decides when the series is too short to
// draw, so a fresh dashboard just shows the number.
function MetricCell({
  value,
  series,
  label,
}: {
  value: string
  series: bigint[]
  label: string
}) {
  return (
    <span className="system-dashboard-metric">
      {value}
      <Sparkline values={series} title={label} />
    </span>
  )
}

// rollUp reduces a stream's consumer groups to the single line shown on the
// stream row: consumer counts, pending and lag add up across groups, and the
// oldest-pending age is the worst one — the number an operator reacts to.
function rollUp(groups: BusGroupStat[]) {
  return groups.reduce(
    (acc, group) => ({
      consumers: acc.consumers + Number(group.consumers),
      pending: acc.pending + Number(group.pending),
      lag: acc.lag + Number(group.lag),
      oldestPendingAge: Math.max(acc.oldestPendingAge, Number(group.oldestPendingAgeSeconds)),
    }),
    { consumers: 0, pending: 0, lag: 0, oldestPendingAge: 0 },
  )
}

function StreamsTable({ streams }: { streams: BusStreamStat[] }) {
  const columns: ColumnsType<BusStreamStat> = [
    {
      title: 'Stream',
      dataIndex: 'stream',
      render: (_, stream) => (
        <span className="system-dashboard-mono">{stream.stream}</span>
      ),
    },
    {
      title: 'Depth',
      align: 'right',
      render: (_, stream) => {
        if (stream.error) {
          return (
            <Tooltip title={stream.error}>
              <span className="system-dashboard-stream-error">unavailable</span>
            </Tooltip>
          )
        }
        if (!stream.exists) {
          return <Tag>not created yet</Tag>
        }
        return (
          <MetricCell
            value={Number(stream.length).toLocaleString()}
            series={stream.lengthSeries}
            label="Depth trend"
          />
        )
      },
    },
    {
      title: (
        <Tooltip title="Entries added to the stream since the previous sample (~2s apart)">
          Publish rate
        </Tooltip>
      ),
      align: 'right',
      render: (_, stream) =>
        stream.exists && !stream.error ? (
          <MetricCell
            value={Number(stream.publishRate).toLocaleString()}
            series={stream.publishRateSeries}
            label="Publish rate trend"
          />
        ) : (
          '—'
        ),
    },
    {
      title: 'Consumers',
      align: 'right',
      render: (_, stream) =>
        stream.exists && !stream.error ? rollUp(stream.groups).consumers.toLocaleString() : '—',
    },
    {
      title: 'Pending',
      align: 'right',
      render: (_, stream) =>
        stream.exists && !stream.error ? rollUp(stream.groups).pending.toLocaleString() : '—',
    },
    {
      title: 'Lag',
      align: 'right',
      render: (_, stream) =>
        stream.exists && !stream.error ? rollUp(stream.groups).lag.toLocaleString() : '—',
    },
    {
      title: 'Oldest pending',
      align: 'right',
      render: (_, stream) =>
        stream.exists && !stream.error
          ? formatDuration(rollUp(stream.groups).oldestPendingAge)
          : '—',
    },
  ]

  return (
    <Table
      size="small"
      rowKey="stream"
      pagination={false}
      columns={columns}
      dataSource={streams}
      expandable={{
        rowExpandable: (stream) => stream.exists && stream.groups.length > 0,
        expandedRowRender: (stream) => <GroupsTable groups={stream.groups} />,
      }}
      locale={{ emptyText: 'No durable streams are registered.' }}
    />
  )
}

function GroupsTable({ groups }: { groups: BusGroupStat[] }) {
  const columns: ColumnsType<BusGroupStat> = [
    {
      title: 'Consumer group',
      dataIndex: 'name',
      render: (_, group) => <span className="system-dashboard-mono">{group.name}</span>,
    },
    {
      title: 'Consumers',
      align: 'right',
      render: (_, group) => Number(group.consumers).toLocaleString(),
    },
    {
      title: 'Pending',
      align: 'right',
      render: (_, group) => (
        <MetricCell
          value={Number(group.pending).toLocaleString()}
          series={group.pendingSeries}
          label="Pending trend"
        />
      ),
    },
    {
      title: 'Lag',
      align: 'right',
      render: (_, group) => (
        <MetricCell
          value={Number(group.lag).toLocaleString()}
          series={group.lagSeries}
          label="Lag trend"
        />
      ),
    },
    {
      title: (
        <Tooltip title="Entries read by the group since the previous sample (~2s apart)">
          Consume rate
        </Tooltip>
      ),
      align: 'right',
      render: (_, group) => (
        <MetricCell
          value={Number(group.consumeRate).toLocaleString()}
          series={group.consumeRateSeries}
          label="Consume rate trend"
        />
      ),
    },
    {
      title: 'Oldest pending',
      align: 'right',
      render: (_, group) => formatDuration(Number(group.oldestPendingAgeSeconds)),
    },
    {
      title: 'Last delivered',
      align: 'right',
      render: (_, group) => (
        <span className="system-dashboard-mono">{group.lastDeliveredId || '—'}</span>
      ),
    },
  ]

  return (
    <Table
      size="small"
      rowKey="name"
      pagination={false}
      columns={columns}
      dataSource={groups}
      className="system-dashboard-groups"
    />
  )
}

function ChannelsTable({ channels }: { channels: BusChannelStat[] }) {
  const columns: ColumnsType<BusChannelStat> = [
    {
      title: 'Channel',
      dataIndex: 'channel',
      render: (_, channel) => (
        <span className="system-dashboard-mono">{channel.channel}</span>
      ),
    },
    {
      title: 'Subscribers',
      align: 'right',
      render: (_, channel) => Number(channel.subscribers).toLocaleString(),
    },
  ]

  return (
    <Table
      size="small"
      rowKey="channel"
      pagination={false}
      columns={columns}
      dataSource={channels}
      locale={{ emptyText: 'Channel detail lands in a later ticket.' }}
    />
  )
}

// formatDuration renders a whole-second span as the largest unit that keeps
// it a small number: an uptime of days, a pending age of minutes. Zero and
// nonsense both read as an em dash.
function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '—'
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`
  return `${Math.floor(seconds / 86400)}d`
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)}${units[unit]}`
}

export function SystemDashboardSidebar() {
  return (
    <div className="system-dashboard-sidebar">
      <Alert
        type="info"
        message="One sampler, many viewers"
        description="A single server-side sampler polls Redis on a fixed cadence into one shared snapshot. Every open dashboard reads that same snapshot over a stream, so a second viewer adds no load on Redis."
      />

      <Alert
        type="info"
        message="Two transports"
        description={
          <>
            <p>
              <strong>Streams</strong> are durable. An event stays on the stream until a
              consumer group acknowledges it, so a listener that is down loses nothing — the
              backlog waits for it.
            </p>
            <p>
              <strong>Pub/Sub</strong> is not. A message goes to whoever is connected at that
              instant and is then gone, which is why those channels report subscribers but no
              depth.
            </p>
          </>
        }
      />
    </div>
  )
}
