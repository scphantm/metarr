import { useEffect, useState } from 'react'
import { Alert, Badge, Col, Row as AntRow, Table, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { timestampDate } from '@bufbuild/protobuf/wkt'

import { useBusSnapshot, useBusSnapshotStreamStatus } from '../../api/queries'
import type { StreamStatus } from '../../api/streams'
import type {
  BusChannelStat,
  BusServerInfo,
  BusStreamStat,
} from '../../gen/metarr/v1/stats_pb'
import { Card } from '../../components/Card'
import { PageError, PageLoading } from '../../components/PageState'
import { PageHeader } from '../../layout/AppShell'
import './SystemDashboardPage.css'

/*
 * The system dashboard — the landing screen.
 *
 * A single server-side sampler polls Redis on a fixed cadence into one shared
 * snapshot and fans it out here; opening a second dashboard adds no Redis
 * load. This walking skeleton renders the six server tiles live off that
 * stream. The stream and channel tables arrive in later tickets and render
 * empty until then.
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
          description="Durable Redis Streams. Events sit on a stream until a consumer group acknowledges them, so depth and pending are real counts."
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

  const tiles: { label: string; value: string; field?: string }[] = [
    { label: 'Version', value: server.version || '—', field: 'version' },
    {
      label: 'Uptime',
      value: formatUptime(Number(server.uptimeSeconds)),
      field: 'uptime_seconds',
    },
    {
      label: 'Connected clients',
      value: Number(server.connectedClients).toLocaleString(),
      field: 'connected_clients',
    },
    {
      label: 'Memory used',
      value: server.usedMemoryHuman || formatBytes(Number(server.usedMemory)),
      field: 'used_memory',
    },
    {
      label: 'Ops / second',
      value: Number(server.opsPerSecond).toLocaleString(),
      field: 'ops_per_second',
    },
    {
      label: 'Keys',
      value: Number(server.totalKeys).toLocaleString(),
      field: 'total_keys',
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
              ) : null}
            </div>
          </Col>
        )
      })}
    </AntRow>
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
      render: (_, stream) => (stream.exists ? Number(stream.length).toLocaleString() : '—'),
    },
  ]

  return (
    <Table
      size="small"
      rowKey="stream"
      pagination={false}
      columns={columns}
      dataSource={streams}
      locale={{ emptyText: 'Stream detail lands in a later ticket.' }}
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

function formatUptime(seconds: number): string {
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
