import { useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'

import { queryKeys, useChatMessages, useSendChatMessage } from '../api/queries'
import type { ChatMessage, ChatToolCall } from '../api/types'
import { useActivePageContext } from '../pagecontext/PageContextRegistry'
import { useChatStream } from './useChatStream'

const SESSION_STORAGE_KEY = 'metarr.chatbot.sessionId'

// One conversation persists across reloads and navigation (the widget
// itself already survives navigation by mounting above <Routes> — see
// App.tsx — this is what makes it survive a full page reload too).
function getOrCreateSessionId(): string {
  let id = localStorage.getItem(SESSION_STORAGE_KEY)
  if (!id) {
    id = crypto.randomUUID()
    localStorage.setItem(SESSION_STORAGE_KEY, id)
  }
  return id
}

export type StreamingMessage = {
  messageId: string
  text: string
  done: boolean
  error: string | null
  toolCall: ChatToolCall | null
}

/**
 * Ties together the active page's context, sending a message, and
 * streaming its reply. The in-flight assistant message is rendered from
 * useChatStream's live state (not the messages list, which still shows it
 * as "pending" with no text until the stream finishes and the list is
 * refetched) — displayMessages filters that placeholder row out so it's
 * never shown twice.
 */
export function useChatSession() {
  const sessionId = useMemo(() => getOrCreateSessionId(), [])
  const activePageContext = useActivePageContext()
  const queryClient = useQueryClient()

  const messagesQuery = useChatMessages(sessionId)
  const sendMutation = useSendChatMessage()

  const [streamingMessageId, setStreamingMessageId] = useState<string | null>(null)
  const stream = useChatStream(streamingMessageId)

  // Once the stream finishes, refetch so the persisted, final version of
  // the message (and the session's updated last-message time) replaces the
  // live-streamed placeholder. No setState here deliberately — streaming
  // (below) already stops representing this message once stream.done is
  // true, by checking stream.done directly rather than needing
  // streamingMessageId cleared back to null.
  useEffect(() => {
    if (!streamingMessageId || !stream.done) return
    void queryClient.invalidateQueries({ queryKey: queryKeys.chatMessages(sessionId) })
    void queryClient.invalidateQueries({ queryKey: queryKeys.chatSessions })
  }, [stream.done, streamingMessageId, sessionId, queryClient])

  async function send(text: string) {
    const trimmed = text.trim()
    if (!trimmed) return

    const result = await sendMutation.mutateAsync({
      session_id: sessionId,
      page_key: activePageContext?.pageKey,
      context: activePageContext?.collect(),
      text: trimmed,
    })
    void queryClient.invalidateQueries({ queryKey: queryKeys.chatMessages(sessionId) })
    setStreamingMessageId(result.message_id)
  }

  // Excludes the message currently being actively streamed (not just
  // "has a streamingMessageId" — once stream.done flips true it stops being
  // excluded, so the finalized version from the invalidated query above
  // takes over from the live bubble rather than the row vanishing).
  const messages: ChatMessage[] = (messagesQuery.data ?? []).filter(
    (message) => !(message.id === streamingMessageId && !stream.done),
  )

  const streaming: StreamingMessage | null =
    streamingMessageId && !stream.done
      ? {
          messageId: streamingMessageId,
          text: stream.text,
          done: stream.done,
          error: stream.error,
          toolCall: stream.toolCall,
        }
      : null

  return {
    sessionId,
    messages,
    messagesLoading: messagesQuery.isLoading,
    streaming,
    hasActivePageContext: activePageContext !== null,
    send,
    sending: sendMutation.isPending,
    sendError: sendMutation.error,
  }
}
