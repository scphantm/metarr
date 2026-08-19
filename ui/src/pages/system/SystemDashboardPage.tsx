import { useRedisStats } from '../../api/queries'
import { useSocketStatus } from '../../api/useTopic'
import type {
  PubSubChannelStat,
  RedisStreamStat,
  RedisServerInfo,
} from '../../api/types'
import { Card } from '../../components/Card'
import { Spinner } from '../../components/SaveState'
import { PageHeader } from '../../layout/AppShell'

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
  const socketStatus = useSocketStatus()

  // Only a failure with nothing cached is fatal to the page. Once a snapshot
  // has arrived, a dropped socket keeps showing it and says so, because a
  // blank dashboard reads as "nothing is running" rather than "I can't see".
  if (stats.error && !stats.data) {
    return (
      <>
        <PageHeader title="System" />
        <div className="px-6 py-5">
          <p className="rounded border border-red/40 bg-red/10 px-4 py-3 text-sm text-red">
            {stats.error instanceof Error
              ? stats.error.message
              : String(stats.error)}
          </p>
        </div>
      </>
    )
  }

  if (!stats.data) {
    return (
      <>
        <PageHeader title="System" />
        <div className="flex items-center gap-2 px-6 py-5 text-sm text-ink-muted">
          <Spinner />
          Connecting to Redis…
        </div>
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

      <div className="flex flex-col gap-5 px-6 py-5">
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

        <p className="text-xs text-ink-muted">
          Last collected{' '}
          {new Date(stats.data.collected_at).toLocaleTimeString()}
        </p>
      </div>
    </>
  )
}

// The connection state is worth showing plainly: a frozen dashboard and idle
// infrastructure look identical otherwise.
function ConnectionIndicator({ status }: { status: string }) {
  const label =
    status === 'open' ? 'Live' : status === 'connecting' ? 'Connecting' : 'Stale'
  const tone =
    status === 'open'
      ? 'text-green'
      : status === 'connecting'
        ? 'text-yellow'
        : 'text-orange'

  return (
    <span className={`flex items-center gap-1.5 text-xs ${tone}`}>
      <span aria-hidden="true">●</span>
      {label}
    </span>
  )
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
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
      {tiles.map((tile) => (
        <div
          key={tile.label}
          className="rounded-lg border border-edge bg-surface px-4 py-3"
        >
          <div className="text-2xl font-semibold text-ink-strong tabular-nums">
            {tile.value}
          </div>
          <div className="mt-0.5 text-xs text-ink-muted">{tile.label}</div>
        </div>
      ))}
    </div>
  )
}

function StreamsTable({ streams }: { streams: RedisStreamStat[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-edge text-left text-xs text-ink-muted uppercase">
            <th className="py-2 pr-4 font-medium">Stream</th>
            <th className="py-2 pr-4 text-right font-medium">Depth</th>
            <th className="py-2 pr-4 text-right font-medium">Pending</th>
            <th className="py-2 pr-4 text-right font-medium">Consumers</th>
            <th className="py-2 text-right font-medium">Lag</th>
          </tr>
        </thead>
        <tbody>
          {streams.map((stream) => {
            // Every stream here has exactly one consumer group, but the shape
            // allows more, so these total across whatever is present.
            const pending = stream.groups.reduce((sum, g) => sum + g.pending, 0)
            const consumers = stream.groups.reduce(
              (sum, g) => sum + g.consumers,
              0,
            )
            const lag = stream.groups.reduce((sum, g) => sum + g.lag, 0)

            return (
              <tr
                key={stream.stream}
                className="border-b border-edge/60 last:border-b-0"
              >
                <td className="py-2 pr-4">
                  <div className="font-mono text-ink-strong">
                    {stream.stream}
                  </div>
                  {stream.error ? (
                    <div className="text-xs text-red">{stream.error}</div>
                  ) : !stream.exists ? (
                    <div className="text-xs text-ink-muted">
                      not created yet — no listener has subscribed
                    </div>
                  ) : null}
                </td>
                <td className="py-2 pr-4 text-right tabular-nums text-ink-strong">
                  {stream.exists ? stream.length.toLocaleString() : '—'}
                </td>
                <td className="py-2 pr-4 text-right tabular-nums">
                  {/* Colour alone never carries the state — the word does the
                      work and the tone reinforces it. */}
                  {pending > 0 ? (
                    <span className="text-yellow">
                      {pending.toLocaleString()} pending
                    </span>
                  ) : (
                    <span className="text-ink-muted">0</span>
                  )}
                </td>
                <td className="py-2 pr-4 text-right tabular-nums text-ink">
                  {stream.exists ? consumers.toLocaleString() : '—'}
                </td>
                <td className="py-2 text-right tabular-nums text-ink">
                  {stream.exists ? lag.toLocaleString() : '—'}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

function ChannelsTable({ channels }: { channels: PubSubChannelStat[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-edge text-left text-xs text-ink-muted uppercase">
            <th className="py-2 pr-4 font-medium">Channel</th>
            <th className="py-2 pr-4 text-right font-medium">Subscribers</th>
            <th className="py-2 text-right font-medium">Depth</th>
          </tr>
        </thead>
        <tbody>
          {channels.map((channel) => (
            <tr
              key={channel.channel}
              className="border-b border-edge/60 last:border-b-0"
            >
              <td className="py-2 pr-4">
                <span className="font-mono text-ink-strong">
                  {channel.channel}
                </span>
                {!channel.known ? (
                  <span className="ml-2 rounded bg-surface-hover px-1.5 py-0.5 text-xs text-ink-muted">
                    transient
                  </span>
                ) : null}
              </td>
              <td className="py-2 pr-4 text-right tabular-nums">
                {channel.subscribers > 0 ? (
                  <span className="text-ink-strong">
                    {channel.subscribers.toLocaleString()}
                  </span>
                ) : (
                  <span className="text-orange">0 — no listener</span>
                )}
              </td>
              <td className="py-2 text-right text-ink-muted">—</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
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
    <section>
      <h2 className="mb-2 text-xs font-semibold tracking-wide text-ink-muted uppercase">
        Two transports
      </h2>
      <div className="rounded border border-edge bg-surface px-3 py-2.5 text-xs leading-relaxed text-ink-muted">
        <p>
          <span className="text-ink">Streams</span> are durable. An event stays
          on the stream until a consumer group acknowledges it, so a listener
          that is down loses nothing — the backlog waits for it.
        </p>
        <p className="mt-2">
          <span className="text-ink">Pub/Sub</span> is not. A message goes to
          whoever is connected at that instant and is then gone, which is why
          those channels report subscribers but no depth.
        </p>
      </div>

      <h2 className="mt-6 mb-2 text-xs font-semibold tracking-wide text-ink-muted uppercase">
        Reading the numbers
      </h2>
      <div className="rounded border border-edge bg-surface px-3 py-2.5 text-xs leading-relaxed text-ink-muted">
        <p>
          Streams are not trimmed, so depth climbing steadily is expected
          rather than a symptom. <span className="text-ink">Pending</span> is
          the number delivered but not yet acknowledged — that one staying
          above zero is worth looking into.
        </p>
      </div>
    </section>
  )
}
