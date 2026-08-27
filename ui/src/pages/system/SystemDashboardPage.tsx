import { Alert, Badge, Col, Row as AntRow, Table, Tag, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'

import { useRedisStats, useRedisStatsStreamStatus } from '../../api/queries'
import type {
  PubSubChannelStat,
  RedisStreamStat,
  RedisServerInfo,
} from '../../api/types'
import { Card } from '../../components/Card'
import { PageError, PageLoading } from '../../components/PageState'
import { PageHeader } from '../../layout/AppShell'
import './SystemDashboardPage.css'

/*
 * The system dashboard.
 *
 * Deliberately tables and stat tiles rather than charts: three streams
 * carrying four measures each is table data, and the server counters are a
 * handful of headline numbers. A three-bar bar chart would say less than the
 * numbers themselves.
 */
export function SystemDashboardPage() {
  const stats = useRedisStats()
  const socketStatus = useRedisStatsStreamStatus()

  // Only a failure with nothing cached is fatal to the page. Once a snapshot
  // has arrived, a dropped socket keeps showing it and says so, because a
  // blank dashboard reads as "nothing is running" rather than "I can't see".
  if (stats.error && !stats.data) {
    return (
      <>
        <PageHeader title="System" />
        <PageError error={stats.error} />
      </>
    )
  }

  if (!stats.data) {
    return (
      <>
        <PageHeader title="System" />
        <PageLoading>Connecting to Redis…</PageLoading>
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="System"
        description="Live statistics for the Redis instance behind the event system."
        actions={<ConnectionIndicator status={socketStatus} />}
      />

      <div className="page-body">
        <ServerTiles server={stats.data.server} />

        <Card
          title="Event streams"
          description="Durable Redis Streams. Events sit on a stream until a consumer group acknowledges them, so depth and pending are real counts."
        >
          <StreamsTable streams={stats.data.streams} />
        </Card>

        <Card
          title="Pub/Sub channels"
          description="Redis Pub/Sub delivers to whoever is connected at that instant and keeps nothing, so these channels have subscribers but no depth to report."
        >
          <ChannelsTable channels={stats.data.pubsub} />
        </Card>

        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          Last collected {new Date(stats.data.collected_at).toLocaleTimeString()}
        </Typography.Text>
      </div>
    </>
  )
}

// The connection state is worth showing plainly: a frozen dashboard and idle
// infrastructure look identical otherwise.
function ConnectionIndicator({ status }: { status: string }) {
  const label =
    status === 'open' ? 'Live' : status === 'connecting' ? 'Connecting' : 'Stale'
  const badgeStatus =
    status === 'open' ? 'success' : status === 'connecting' ? 'processing' : 'warning'

  return <Badge status={badgeStatus} text={label} />
}

function ServerTiles({ server }: { server: RedisServerInfo }) {
  const tiles = [
    { label: 'Connected clients', value: server.connected_clients.toLocaleString() },
    { label: 'Memory used', value: server.used_memory_human || '—' },
    { label: 'Ops / second', value: server.ops_per_second.toLocaleString() },
    { label: 'Keys', value: server.total_keys.toLocaleString() },
    { label: 'Uptime', value: formatUptime(server.uptime_seconds) },
  ]

  return (
    <AntRow gutter={[12, 12]}>
      {tiles.map((tile) => (
        <Col key={tile.label} xs={12} sm={8} lg={4}>
          <div className="system-dashboard-tile">
            <div className="system-dashboard-tile-value">{tile.value}</div>
            <div className="system-dashboard-tile-label">{tile.label}</div>
          </div>
        </Col>
      ))}
    </AntRow>
  )
}

function StreamsTable({ streams }: { streams: RedisStreamStat[] }) {
  const columns: ColumnsType<RedisStreamStat> = [
    {
      title: 'Stream',
      dataIndex: 'stream',
      render: (_, stream) => (
        <div>
          <span className="system-dashboard-mono">{stream.stream}</span>
          {stream.error ? (
            <div className="system-dashboard-error-note">{stream.error}</div>
          ) : !stream.exists ? (
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
              not created yet — no listener has subscribed
            </Typography.Text>
          ) : null}
        </div>
      ),
    },
    {
      title: 'Depth',
      align: 'right',
      render: (_, stream) => (stream.exists ? stream.length.toLocaleString() : '—'),
    },
    {
      title: 'Pending',
      align: 'right',
      render: (_, stream) => {
        const pending = stream.groups.reduce((sum, g) => sum + g.pending, 0)
        // Colour alone never carries the state — the word does the work and
        // the tone reinforces it.
        return pending > 0 ? (
          <span style={{ color: 'var(--color-yellow)' }}>{pending.toLocaleString()} pending</span>
        ) : (
          <Typography.Text type="secondary">0</Typography.Text>
        )
      },
    },
    {
      title: 'Consumers',
      align: 'right',
      render: (_, stream) =>
        stream.exists
          ? stream.groups.reduce((sum, g) => sum + g.consumers, 0).toLocaleString()
          : '—',
    },
    {
      title: 'Lag',
      align: 'right',
      render: (_, stream) =>
        stream.exists ? stream.groups.reduce((sum, g) => sum + g.lag, 0).toLocaleString() : '—',
    },
  ]

  return (
    <Table
      size="small"
      rowKey="stream"
      pagination={false}
      columns={columns}
      dataSource={streams}
    />
  )
}

function ChannelsTable({ channels }: { channels: PubSubChannelStat[] }) {
  const columns: ColumnsType<PubSubChannelStat> = [
    {
      title: 'Channel',
      dataIndex: 'channel',
      render: (_, channel) => (
        <>
          <span className="system-dashboard-mono">{channel.channel}</span>
          {!channel.known ? <Tag style={{ marginLeft: 8 }}>transient</Tag> : null}
        </>
      ),
    },
    {
      title: 'Subscribers',
      align: 'right',
      render: (_, channel) =>
        channel.subscribers > 0 ? (
          channel.subscribers.toLocaleString()
        ) : (
          <span style={{ color: 'var(--color-orange)' }}>0 — no listener</span>
        ),
    },
    {
      title: 'Depth',
      align: 'right',
      render: () => <Typography.Text type="secondary">—</Typography.Text>,
    },
  ]

  return (
    <Table
      size="small"
      rowKey="channel"
      pagination={false}
      columns={columns}
      dataSource={channels}
    />
  )
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`
  return `${Math.floor(seconds / 86400)}d`
}

export function SystemDashboardSidebar() {
  return (
    <div className="system-dashboard-sidebar">
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

      <Alert
        type="info"
        message="Reading the numbers"
        description="Streams are not trimmed, so depth climbing steadily is expected rather than a symptom. Pending is the number delivered but not yet acknowledged — that one staying above zero is worth looking into."
      />
    </div>
  )
}
