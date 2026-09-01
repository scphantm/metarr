import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'

import { SystemDashboardPage } from '../SystemDashboardPage'

const useBusSnapshot = vi.fn()
const useBusSnapshotStreamStatus = vi.fn()

vi.mock('../../../api/queries', () => ({
  useBusSnapshot: () => useBusSnapshot(),
  useBusSnapshotStreamStatus: () => useBusSnapshotStreamStatus(),
}))

function snapshot(overrides: Record<string, unknown> = {}) {
  return {
    collectedAt: { seconds: BigInt(Math.floor(Date.now() / 1000)), nanos: 0 },
    streams: [],
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
})
