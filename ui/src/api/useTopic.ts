import { useEffect, useSyncExternalStore } from 'react'
import { useQueryClient } from '@tanstack/react-query'

import {
  getSocketStatus,
  onSocketStatus,
  subscribe,
  type SocketStatus,
} from './socket'

/*
 * Streaming pushed into the query cache.
 *
 * Components keep using useQuery exactly as they do for a plain REST read;
 * this hook just keeps that cache entry fresh from the socket instead of from
 * a refetch. The query's own queryFn still runs once for the first paint and
 * as a fallback when the socket cannot connect.
 */

/** Subscribes to topic and writes each frame into queryKey's cache entry. */
export function useTopic(topic: string, queryKey: readonly unknown[]): void {
  const queryClient = useQueryClient()

  // queryKey is a fresh array literal on every render, so the effect keys off
  // its serialized form rather than the array identity.
  const serializedKey = JSON.stringify(queryKey)

  useEffect(() => {
    return subscribe(topic, (payload) => {
      queryClient.setQueryData(JSON.parse(serializedKey), payload)
    })
  }, [topic, serializedKey, queryClient])
}

/** The live connection status, for showing whether a stream is still moving. */
export function useSocketStatus(): SocketStatus {
  return useSyncExternalStore(onSocketStatus, getSocketStatus, () => 'closed')
}
