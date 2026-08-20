import { useEffect, useRef, useState } from 'react'

import { Button } from '../components/Card'
import { Spinner } from '../components/SaveState'
import type { ChatMessage, ContextSentRecord } from '../api/types'
import { ContextSentModal } from './ContextSentModal'
import { ProposedEditCard } from './ProposedEditCard'
import { useChatSession, type StreamingMessage } from './useChatSession'

export function ChatPanel() {
  const {
    messages,
    messagesLoading,
    streaming,
    hasActivePageContext,
    send,
    sending,
    sendError,
  } = useChatSession()
  const [input, setInput] = useState('')
  const [openContext, setOpenContext] = useState<ContextSentRecord | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: 'end' })
  }, [messages.length, streaming?.text])

  async function handleSend() {
    const text = input
    setInput('')
    await send(text)
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 overflow-y-auto px-3 py-3">
        <div className="flex flex-col gap-3">
          {messagesLoading ? (
            <div className="flex items-center gap-2 text-sm text-ink-muted">
              <Spinner />
              Loading…
            </div>
          ) : null}
          {!messagesLoading && messages.length === 0 && !streaming ? (
            <p className="text-sm text-ink-muted">Ask me anything.</p>
          ) : null}
          {messages.map((message) => (
            <MessageRow key={message.id} message={message} onOpenContext={setOpenContext} />
          ))}
          {streaming ? <StreamingRow streaming={streaming} /> : null}
          <div ref={bottomRef} />
        </div>
      </div>

      {hasActivePageContext ? (
        <div className="border-t border-edge px-3 py-1.5 text-[11px] text-cyan">
          Context from this page will be sent with your next message.
        </div>
      ) : null}

      <div className="flex items-center gap-2 border-t border-edge px-3 py-2">
        <input
          type="text"
          value={input}
          onChange={(event) => setInput(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !sending && input.trim()) {
              void handleSend()
            }
          }}
          placeholder="Message…"
          className="flex-1 rounded border border-edge bg-canvas px-2.5 py-1.5 text-sm text-ink-strong"
        />
        <Button variant="primary" onClick={() => void handleSend()} disabled={sending || !input.trim()}>
          Send
        </Button>
      </div>
      {sendError ? (
        <p className="px-3 pb-2 text-xs text-red">
          {sendError instanceof Error ? sendError.message : String(sendError)}
        </p>
      ) : null}

      {openContext ? (
        <ContextSentModal record={openContext} onClose={() => setOpenContext(null)} />
      ) : null}
    </div>
  )
}

function MessageRow({
  message,
  onOpenContext,
}: {
  message: ChatMessage
  onOpenContext: (record: ContextSentRecord) => void
}) {
  const isUser = message.role === 'user'
  const body =
    message.text ||
    (message.status === 'pending' ? '…' : message.status === 'failed' ? 'Something went wrong.' : '')

  return (
    <div className={`flex flex-col gap-1 ${isUser ? 'items-end' : 'items-start'}`}>
      {body ? (
        <div
          className={`max-w-[85%] rounded-lg px-3 py-2 text-sm ${
            isUser ? 'bg-blue text-canvas' : 'bg-surface-hover text-ink-strong'
          } ${message.status === 'failed' ? 'text-red' : ''}`}
        >
          {body}
        </div>
      ) : null}
      {message.tool_call ? <ProposedEditCard toolCall={message.tool_call} /> : null}
      {message.context_sent ? (
        <button
          type="button"
          onClick={() => onOpenContext(message.context_sent!)}
          aria-label="Show context sent with this message"
          title="Context was sent with this message"
          className="h-3.5 w-3.5 shrink-0 rounded-full bg-cyan/70 transition-colors hover:bg-cyan"
        />
      ) : null}
    </div>
  )
}

function StreamingRow({ streaming }: { streaming: StreamingMessage }) {
  const body = streaming.text || (streaming.error ? '' : streaming.toolCall ? '' : '…')

  return (
    <div className="flex flex-col items-start gap-1">
      {body || streaming.error ? (
        <div className="max-w-[85%] rounded-lg bg-surface-hover px-3 py-2 text-sm text-ink-strong">
          {body}
          {streaming.error ? <span className="text-red">{streaming.error}</span> : null}
        </div>
      ) : null}
      {streaming.toolCall ? <ProposedEditCard toolCall={streaming.toolCall} /> : null}
    </div>
  )
}
