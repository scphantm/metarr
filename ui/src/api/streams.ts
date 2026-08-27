/*
 * The streaming client — replaces socket.ts + useTopic.ts.
 *
 * Browser gRPC-Web has no client-streaming/bidi, so the old model (one
 * WebSocket carrying every topic as a subscription over it) can't be
 * replicated 1:1. Instead there is one singleton per server-streaming RPC
 * (stats.redis -> StatsService.Stream, agents.presence ->
 * AgentService.StreamPresence, logging.tail -> LoggingService.StreamTail),
 * each refcounted so multiple components mounting the same stream still
 * open only one underlying connection, with the same exponential-backoff
 * reconnect socket.ts had.
 *
 * useStream keeps the identical queryClient.setQueryData push-not-invalidate
 * model useTopic.ts had: components keep using useQuery exactly as they do
 * for a plain read; this just keeps that cache entry fresh from the stream
 * instead of a refetch.
 */

import { useEffect, useSyncExternalStore } from 'react'
import { useQueryClient } from '@tanstack/react-query'

export type StreamStatus = 'connecting' | 'open' | 'closed'

const reconnectBaseDelay = 1000
const reconnectMaxDelay = 30000

/** One server-streaming RPC, refcounted across every component watching it. */
class Stream<T> {
  private readonly subscribers = new Set<(value: T) => void>()
  private readonly statusHandlers = new Set<(status: StreamStatus) => void>()
  private status: StreamStatus = 'closed'
  private controller: AbortController | null = null
  private reconnectAttempts = 0
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private generation = 0
  private readonly open: (signal: AbortSignal) => AsyncIterable<T>

  constructor(open: (signal: AbortSignal) => AsyncIterable<T>) {
    this.open = open
  }

  getStatus = (): StreamStatus => this.status

  onStatus = (handler: (status: StreamStatus) => void): (() => void) => {
    this.statusHandlers.add(handler)
    return () => this.statusHandlers.delete(handler)
  }

  subscribe(handler: (value: T) => void): () => void {
    this.subscribers.add(handler)
    if (this.subscribers.size === 1) this.start()

    return () => {
      this.subscribers.delete(handler)
      if (this.subscribers.size === 0) this.stop()
    }
  }

  /** Drops every subscriber and closes the connection — called on sign-out. */
  reset(): void {
    this.subscribers.clear()
    this.stop()
  }

  private setStatus(next: StreamStatus): void {
    if (this.status === next) return
    this.status = next
    this.statusHandlers.forEach((handler) => handler(next))
  }

  private start(): void {
    if (this.controller) return
    const generation = ++this.generation
    void this.run(generation)
  }

  private async run(generation: number): Promise<void> {
    const controller = new AbortController()
    this.controller = controller
    this.setStatus('connecting')

    try {
      for await (const value of this.open(controller.signal)) {
        if (generation !== this.generation) return
        this.reconnectAttempts = 0
        this.setStatus('open')
        this.subscribers.forEach((handler) => handler(value))
      }
    } catch {
      // Falls through to the reconnect scheduling below — an aborted call
      // (intentional stop) and a dropped connection both land here, and the
      // generation check keeps an aborted call's rejection from scheduling
      // a reconnect after a newer run has already started.
    }

    if (generation !== this.generation || controller.signal.aborted) return
    this.controller = null
    this.setStatus('closed')
    this.scheduleReconnect(generation)
  }

  private scheduleReconnect(generation: number): void {
    if (this.subscribers.size === 0) return

    const delay = Math.min(reconnectBaseDelay * 2 ** this.reconnectAttempts, reconnectMaxDelay)
    this.reconnectAttempts += 1

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      if (generation === this.generation) void this.run(generation)
    }, delay)
  }

  private stop(): void {
    this.generation += 1
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    this.reconnectAttempts = 0
    this.controller?.abort()
    this.controller = null
    this.setStatus('closed')
  }
}

/** Subscribes to stream and writes each frame into queryKey's cache entry. */
function useStream<T>(stream: Stream<T>, queryKey: readonly unknown[]): void {
  const queryClient = useQueryClient()
  const serializedKey = JSON.stringify(queryKey)

  useEffect(() => {
    return stream.subscribe((value) => {
      queryClient.setQueryData(JSON.parse(serializedKey) as unknown[], value)
    })
  }, [stream, serializedKey, queryClient])
}

function useStreamStatus<T>(stream: Stream<T>): StreamStatus {
  return useSyncExternalStore(stream.onStatus, stream.getStatus, () => 'closed')
}

/** Transforms each item of a server-streaming RPC's AsyncIterable, so a
 * Stream can decode wire messages (e.g. opaque JSON bytes) into the shape
 * its subscribers actually want. */
async function* mapAsync<I, O>(source: AsyncIterable<I>, transform: (value: I) => O): AsyncIterable<O> {
  for await (const value of source) {
    yield transform(value)
  }
}

export { useStream, useStreamStatus, Stream, mapAsync }

// registry of every live stream, so resetStreams() (called from
// AuthContext.clearSession, same as resetSocket() before it) can drop every
// subscriber and close every connection on sign-out.
const registry = new Set<Stream<unknown>>()

export function registerStream<T>(stream: Stream<T>): Stream<T> {
  registry.add(stream as Stream<unknown>)
  return stream
}

export function resetStreams(): void {
  registry.forEach((stream) => stream.reset())
}
