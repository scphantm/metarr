/*
 * The streaming client.
 *
 * One WebSocket carries every topic the app subscribes to, because a topic is
 * a subscription over the connection rather than a connection of its own. The
 * socket opens on the first subscribe and closes when the last one goes away,
 * so the login screen never opens one.
 *
 * Subscriptions are reference counted locally: several components watching the
 * same topic produce a single server-side subscription, and the server only
 * runs a producer while someone is listening. After a reconnect every live
 * topic is re-sent, so a dropped connection recovers without the callers
 * knowing it happened.
 */

import { getApiKey } from './client'

export type SocketStatus = 'connecting' | 'open' | 'closed'

type ServerMessage = {
  type: 'data' | 'ack' | 'error' | 'pong'
  topic?: string
  payload?: unknown
  error?: string
}

const reconnectBaseDelay = 1000
const reconnectMaxDelay = 30000

let socket: WebSocket | null = null
let status: SocketStatus = 'closed'
let reconnectAttempts = 0
let reconnectTimer: ReturnType<typeof setTimeout> | null = null

// Handlers per topic. The map's size is also the reference count that decides
// whether the server still needs the subscription.
const topicHandlers = new Map<string, Set<(payload: unknown) => void>>()
const statusHandlers = new Set<(status: SocketStatus) => void>()

function socketUrl(): string {
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const key = getApiKey() ?? ''
  // The key goes in the query string because a browser cannot set headers on
  // a WebSocket handshake. The API already accepts it there, and the request
  // logger records only the path, so it does not reach the logs.
  return `${scheme}//${window.location.host}/api/ws?apikey=${encodeURIComponent(key)}`
}

function setStatus(next: SocketStatus): void {
  if (status === next) return
  status = next
  statusHandlers.forEach((handler) => handler(next))
}

export function getSocketStatus(): SocketStatus {
  return status
}

export function onSocketStatus(
  handler: (status: SocketStatus) => void,
): () => void {
  statusHandlers.add(handler)
  return () => statusHandlers.delete(handler)
}

function send(message: { type: string; topic?: string }): void {
  if (socket?.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify(message))
  }
}

function connect(): void {
  if (socket || topicHandlers.size === 0) return

  setStatus('connecting')
  const ws = new WebSocket(socketUrl())
  socket = ws

  ws.onopen = () => {
    reconnectAttempts = 0
    setStatus('open')
    // Re-establish everything the app is currently watching. On a first
    // connect this is the subscribe that prompted the socket to open.
    topicHandlers.forEach((_, topic) => send({ type: 'subscribe', topic }))
  }

  ws.onmessage = (event) => {
    let message: ServerMessage
    try {
      message = JSON.parse(event.data as string) as ServerMessage
    } catch {
      return
    }

    if (message.type === 'data' && message.topic) {
      topicHandlers
        .get(message.topic)
        ?.forEach((handler) => handler(message.payload))
    }
  }

  ws.onclose = () => {
    socket = null
    setStatus('closed')
    scheduleReconnect()
  }

  // onclose always follows onerror, so reconnecting is left to that one path.
  ws.onerror = () => ws.close()
}

function scheduleReconnect(): void {
  if (reconnectTimer !== null || topicHandlers.size === 0) return

  const delay = Math.min(
    reconnectBaseDelay * 2 ** reconnectAttempts,
    reconnectMaxDelay,
  )
  reconnectAttempts += 1

  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    connect()
  }, delay)
}

function disconnectIfIdle(): void {
  if (topicHandlers.size > 0) return

  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  reconnectAttempts = 0

  socket?.close()
  socket = null
  setStatus('closed')
}

/**
 * Starts delivering topic to handler, and returns the function that stops it.
 */
export function subscribe(
  topic: string,
  handler: (payload: unknown) => void,
): () => void {
  let handlers = topicHandlers.get(topic)
  const isFirst = handlers === undefined

  if (!handlers) {
    handlers = new Set()
    topicHandlers.set(topic, handlers)
  }
  handlers.add(handler)

  if (isFirst) {
    // A live socket needs telling about the new topic; a closed one will send
    // every subscription once it opens.
    send({ type: 'subscribe', topic })
  }
  connect()

  return () => {
    const current = topicHandlers.get(topic)
    if (!current) return

    current.delete(handler)
    if (current.size === 0) {
      topicHandlers.delete(topic)
      send({ type: 'unsubscribe', topic })
      disconnectIfIdle()
    }
  }
}

/**
 * Drops every subscription and closes the socket. Called on sign-out, so the
 * next session opens a connection with its own key rather than reusing one
 * authenticated as the previous user.
 */
export function resetSocket(): void {
  topicHandlers.clear()
  statusHandlers.forEach((handler) => handler('closed'))
  disconnectIfIdle()
}
