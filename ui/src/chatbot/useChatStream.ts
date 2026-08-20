import { useEffect, useState } from 'react'

import { getApiKey } from '../api/client'
import type { ChatDelta, ChatToolCall } from '../api/types'

/*
 * A dedicated, per-message WebSocket — not socket.ts's shared,
 * reference-counted connection. That connection multiplexes many durable,
 * shared topics (logs, GPU stats) over one socket, which is the right model
 * for those; a chat reply is the opposite shape — one private, ephemeral
 * stream for one specific message — so it gets its own short-lived
 * connection instead, following the same ?apikey= query-param auth
 * convention socket.ts uses (a browser can't set headers on a WS
 * handshake).
 *
 * Connecting is what actually runs the completion server-side (see
 * handlers.ChatStream's doc comment) — there is nothing to "subscribe" to
 * before this hook mounts.
 */

export type ChatStreamState = {
  text: string
  done: boolean
  error: string | null
  toolCall: ChatToolCall | null
}

const initialState: ChatStreamState = { text: '', done: false, error: null, toolCall: null }

export function useChatStream(messageId: string | null): ChatStreamState {
  const [state, setState] = useState<ChatStreamState>(initialState)
  // Tracks which messageId `state` belongs to, so a change can be detected
  // and reset during render (React's own pattern for "reset state when a
  // prop changes") rather than via a setState call inside an effect.
  const [trackedMessageId, setTrackedMessageId] = useState(messageId)
  if (messageId !== trackedMessageId) {
    setTrackedMessageId(messageId)
    setState(initialState)
  }

  useEffect(() => {
    if (!messageId) return

    const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const key = getApiKey() ?? ''
    const ws = new WebSocket(
      `${scheme}//${window.location.host}/api/chatbot/stream/${encodeURIComponent(messageId)}?apikey=${encodeURIComponent(key)}`,
    )

    ws.onmessage = (event) => {
      let delta: ChatDelta
      try {
        delta = JSON.parse(event.data as string) as ChatDelta
      } catch {
        return
      }
      setState((current) => ({
        text: current.text + (delta.text ?? ''),
        done: current.done || Boolean(delta.done),
        error: delta.error ?? current.error,
        toolCall: delta.tool_call ?? current.toolCall ?? null,
      }))
    }

    ws.onerror = () => {
      setState((current) =>
        current.done ? current : { ...current, done: true, error: current.error ?? 'connection error' },
      )
    }

    return () => ws.close()
  }, [messageId])

  return state
}
