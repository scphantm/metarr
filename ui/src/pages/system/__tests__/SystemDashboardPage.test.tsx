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
) {
  return {
    collectedAt: { seconds: BigInt(Math.floor(Date.now() / 1000)), nanos: 0 },
    streams,
    channels: [],
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
})
