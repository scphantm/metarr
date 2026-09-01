import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'

import { SystemDashboardPage } from '../SystemDashboardPage'

const useBusSnapshot = vi.fn()
const useBusSnapshotStreamStatus = vi.fn()

vi.mock('../../../api/queries', () => ({
  useBusSnapshot: () => useBusSnapshot(),
  useBusSnapshotStreamStatus: () => useBusSnapshotStreamStatus(),
}))

function snapshot(
  overrides: Record<string, unknown> = {},
  streams: Array<Record<string, unknown>> = [],
  channels: Array<Record<string, unknown>> = [],
) {
  return {
    collectedAt: { seconds: BigInt(Math.floor(Date.now() / 1000)), nanos: 0 },
    streams,
    channels,
    server: {
      version: '7.2.4',
      uptimeSeconds: 90_061,
      connectedClients: 6,
      usedMemory: 1_048_576,
      usedMemoryHuman: '1.00M',
      opsPerSecond: 42,
      totalKeys: 17,
      connectedClientsSeries: [],
      usedMemorySeries: [],
      opsPerSecondSeries: [],
      totalKeysSeries: [],
      fieldErrors: {},
      ...overrides,
    },
  }
}

function group(overrides: Record<string, unknown> = {}) {
  return {
    name: 'agent_scan_results_group',
    consumers: 1,
    pending: 2,
    lag: 3,
    lastDeliveredId: '1700000000000-0',
    consumerDetail: [],
    oldestPendingAgeSeconds: 90,
    consumeRate: 4,
    consumersSeries: [],
    pendingSeries: [],
    lagSeries: [],
    oldestPendingAgeSecondsSeries: [],
    consumeRateSeries: [],
    ...overrides,
  }
}

function channel(overrides: Record<string, unknown> = {}) {
  return {
    channel: 'heartbeat.request',
    subscribers: 1,
    known: true,
    expectedIdentities: [],
    flagged: false,
    missingIdentities: [],
    ...overrides,
  }
}

function stream(overrides: Record<string, unknown> = {}) {
  return {
    stream: 'events.agent_scan_results',
    eventName: 'agent.scan_result',
    length: 5,
    exists: true,
    groups: [group()],
    error: '',
    publishRate: 7,
    lengthSeries: [],
    publishRateSeries: [],
    expectedIdentities: [],
    flagged: false,
    missingIdentities: [],
    ...overrides,
  }
}

describe('SystemDashboardPage', () => {
  beforeEach(() => {
    useBusSnapshot.mockReset()
    useBusSnapshotStreamStatus.mockReset()
    useBusSnapshotStreamStatus.mockReturnValue('open')
  })

  it('renders the six server tiles from a snapshot', () => {
    useBusSnapshot.mockReturnValue({
      data: snapshot(),
      error: null,
      dataUpdatedAt: Date.now(),
    })

    render(<SystemDashboardPage />)

    for (const label of [
      'Version',
      'Uptime',
      'Connected clients',
      'Memory used',
      'Ops / second',
      'Keys',
    ]) {
      expect(screen.getByText(label)).toBeDefined()
    }
    expect(screen.getByText('7.2.4')).toBeDefined()
    expect(screen.getByText('6')).toBeDefined()
    expect(screen.getByText('1.00M')).toBeDefined()
    expect(screen.getByText('42')).toBeDefined()
    expect(screen.getByText('17')).toBeDefined()
    expect(screen.getByText('Live')).toBeDefined()
  })

  it('blanks a tile whose field the sampler could not read', () => {
    useBusSnapshot.mockReturnValue({
      data: snapshot({ totalKeys: 0, fieldErrors: { total_keys: 'LOADING' } }),
      error: null,
    })

    render(<SystemDashboardPage />)

    expect(screen.getByText('unavailable')).toBeDefined()
  })

  it('shows a stale badge when the stream is closed', () => {
    useBusSnapshotStreamStatus.mockReturnValue('closed')
    useBusSnapshot.mockReturnValue({
      data: snapshot(),
      error: null,
      dataUpdatedAt: Date.now(),
    })

    render(<SystemDashboardPage />)

    expect(screen.getByText('Stale')).toBeDefined()
  })

  it('waits for the first snapshot before painting tiles', () => {
    useBusSnapshot.mockReturnValue({ data: null, error: null })

    render(<SystemDashboardPage />)

    expect(screen.getByText(/Connecting to the event bus/)).toBeDefined()
  })

  it('renders one row per durable stream with rolled-up group figures', () => {
    useBusSnapshot.mockReturnValue({
      data: snapshot({}, [
        stream({ stream: 'events.system_config_update', groups: [group({ name: 'system_config_update_group', consumers: 1, pending: 0, lag: 0, oldestPendingAgeSeconds: 0 })] }),
        stream({ stream: 'events.agent_node_results', exists: false, groups: [] }),
      ]),
      error: null,
      dataUpdatedAt: Date.now(),
    })

    render(<SystemDashboardPage />)

    expect(screen.getByText('events.system_config_update')).toBeDefined()
    // The reserved node-result stream reads as not-created rather than as an error.
    expect(screen.getByText('events.agent_node_results')).toBeDefined()
    expect(screen.getByText('not created yet')).toBeDefined()
  })

  it('expands a stream row to show its consumer-group rows', () => {
    useBusSnapshot.mockReturnValue({
      data: snapshot({}, [stream()]),
      error: null,
      dataUpdatedAt: Date.now(),
    })

    render(<SystemDashboardPage />)

    // Collapsed: the group name is not shown yet.
    expect(screen.queryByText('agent_scan_results_group')).toBeNull()

    fireEvent.click(screen.getByLabelText('Expand row'))

    // Expanded: the per-group row is now visible, with its own figures.
    expect(screen.getByText('agent_scan_results_group')).toBeDefined()
    expect(screen.getByText('Consumer group')).toBeDefined()
    // The oldest-pending age is rendered as a compact duration (90s -> 1m).
    expect(within(screen.getByText('agent_scan_results_group').closest('tr')!).getByText('1m')).toBeDefined()
  })

  it('shows the publish rate on the stream row', () => {
    useBusSnapshot.mockReturnValue({
      data: snapshot({}, [stream({ publishRate: 12 })]),
      error: null,
      dataUpdatedAt: Date.now(),
    })

    render(<SystemDashboardPage />)

    expect(screen.getByText('Publish rate')).toBeDefined()
    expect(screen.getByText('12')).toBeDefined()
  })

  it('draws an inline sparkline from a metric series with no charting library', () => {
    useBusSnapshot.mockReturnValue({
      data: snapshot({}, [
        stream({ length: 9, lengthSeries: [1n, 4n, 2n, 9n], publishRateSeries: [0n, 1n, 0n, 3n] }),
      ]),
      error: null,
      dataUpdatedAt: Date.now(),
    })

    const view = render(<SystemDashboardPage />)

    const sparklines = view.container.querySelectorAll('svg.system-dashboard-sparkline polyline')
    expect(sparklines.length).toBeGreaterThan(0)
    // The polyline carries one point per sample in the series.
    const depthLine = view.container.querySelector(
      'svg.system-dashboard-sparkline polyline',
    ) as SVGPolylineElement
    expect(depthLine.getAttribute('points')?.trim().split(/\s+/).length).toBe(4)
  })

  it('omits the sparkline until the series has at least two samples', () => {
    useBusSnapshot.mockReturnValue({
      data: snapshot({}, [stream({ lengthSeries: [], publishRateSeries: [5n] })]),
      error: null,
      dataUpdatedAt: Date.now(),
    })

    const view = render(<SystemDashboardPage />)

    expect(view.container.querySelector('svg.system-dashboard-sparkline')).toBeNull()
  })

  it('renders a Pub/Sub channel row with its subscriber count and a declared tag', () => {
    useBusSnapshot.mockReturnValue({
      data: snapshot({}, [], [channel({ channel: 'logs.app', subscribers: 4, known: true })]),
      error: null,
      dataUpdatedAt: Date.now(),
    })

    render(<SystemDashboardPage />)

    const row = screen.getByText('logs.app').closest('tr')!
    expect(within(row).getByText('declared')).toBeDefined()
    expect(within(row).getByText('4')).toBeDefined()
  })

  it('flags a declared channel that has dropped to zero subscribers', () => {
    useBusSnapshot.mockReturnValue({
      data: snapshot({}, [], [
        channel({
          channel: 'heartbeat.request',
          subscribers: 0,
          known: true,
          flagged: true,
          expectedIdentities: ['metarr-server'],
          missingIdentities: ['metarr-server'],
        }),
      ]),
      error: null,
      dataUpdatedAt: Date.now(),
    })

    render(<SystemDashboardPage />)

    const row = screen.getByText('heartbeat.request').closest('tr')!
    expect(within(row).getByText('no subscribers')).toBeDefined()
    expect(row.className).toContain('system-dashboard-row-flagged')
  })

  it('shows a stream row its expected identities and flags a missing one', () => {
    useBusSnapshot.mockReturnValue({
      data: snapshot({}, [
        stream({
          stream: 'events.agent.nas-01.commands',
          exists: false,
          groups: [],
          expectedIdentities: ['metarr-agent-nas-01'],
          flagged: true,
          missingIdentities: ['metarr-agent-nas-01'],
        }),
      ]),
      error: null,
      dataUpdatedAt: Date.now(),
    })

    render(<SystemDashboardPage />)

    const row = screen.getByText('events.agent.nas-01.commands').closest('tr')!
    // The expected identity is shown, and the row is flagged as broken.
    expect(within(row).getAllByText('metarr-agent-nas-01').length).toBeGreaterThan(0)
    expect(within(row).getByText('identity missing')).toBeDefined()
    expect(row.className).toContain('system-dashboard-row-flagged')
  })

  it('lists a per-agent channel row for an offline registered agent', () => {
    useBusSnapshot.mockReturnValue({
      data: snapshot({}, [], [
        channel({
          channel: 'agent.config.changed.nas-01',
          subscribers: 0,
          known: true,
          expectedIdentities: ['metarr-agent-nas-01'],
          flagged: true,
          missingIdentities: ['metarr-agent-nas-01'],
        }),
      ]),
      error: null,
      dataUpdatedAt: Date.now(),
    })

    render(<SystemDashboardPage />)

    const row = screen.getByText('agent.config.changed.nas-01').closest('tr')!
    expect(within(row).getByText('metarr-agent-nas-01')).toBeDefined()
    expect(within(row).getByText('no subscribers')).toBeDefined()
    expect(row.className).toContain('system-dashboard-row-flagged')
  })

  it('distinguishes a transient channel from a declared one', () => {
    useBusSnapshot.mockReturnValue({
      data: snapshot({}, [], [
        channel({ channel: 'heartbeat.request', subscribers: 1, known: true }),
        channel({ channel: 'reply.corr-1234', subscribers: 1, known: false }),
      ]),
      error: null,
      dataUpdatedAt: Date.now(),
    })

    render(<SystemDashboardPage />)

    const transientRow = screen.getByText('reply.corr-1234').closest('tr')!
    expect(within(transientRow).getByText('transient')).toBeDefined()
    expect(transientRow.className).not.toContain('system-dashboard-row-flagged')
  })
})
