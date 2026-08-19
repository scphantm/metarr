/*
 * The HTTP client.
 *
 * Two things about this API shape the whole UI and are worth stating here
 * rather than rediscovering in a component:
 *
 * 1. Every mutation returns 202 Accepted, not 200. The handler fires a
 *    system_config_update event and returns before anything is persisted; a
 *    listener writes it to Mongo afterwards. So a successful response means
 *    "queued", never "saved", and the UI has to confirm by reading back.
 *
 * 2. Errors come back as plain text via Go's http.Error, not as JSON. The
 *    body is the message, and it is usually worth showing verbatim — the
 *    server explains rejections properly ("order 210 is already used by ...").
 */

const apiKeyHeaderName = 'X-Api-Key'
const sessionStorageKey = 'metarr.apiKey'

// ApiError carries the status alongside the server's own message so callers can
// branch on the code without parsing prose.
export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }

  get isUnauthorized(): boolean {
    return this.status === 401
  }

  get isForbidden(): boolean {
    return this.status === 403
  }
}

// The key lives in memory for the session and is mirrored to sessionStorage so
// a page reload does not force a new login. sessionStorage rather than
// localStorage: the key is a session credential and should not outlive the tab.
let apiKey: string | null = readStoredKey()

function readStoredKey(): string | null {
  try {
    return sessionStorage.getItem(sessionStorageKey)
  } catch {
    return null
  }
}

export function getApiKey(): string | null {
  return apiKey
}

export function setApiKey(key: string | null): void {
  apiKey = key
  try {
    if (key) {
      sessionStorage.setItem(sessionStorageKey, key)
    } else {
      sessionStorage.removeItem(sessionStorageKey)
    }
  } catch {
    // Storage refused; the key still works for this page's lifetime.
  }
}

// unauthorizedHandlers lets the auth layer react to a key going stale mid-
// session — a 401 from any request, not just the next explicit auth call.
const unauthorizedHandlers = new Set<() => void>()

export function onUnauthorized(handler: () => void): () => void {
  unauthorizedHandlers.add(handler)
  return () => unauthorizedHandlers.delete(handler)
}

type RequestOptions = {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  body?: unknown
  // Set for the login call, which is one of the three endpoints reachable
  // without a key and must not send a stale one.
  anonymous?: boolean
}

export async function request<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { method = 'GET', body, anonymous = false } = options

  const headers: Record<string, string> = {}
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }
  if (!anonymous && apiKey) {
    headers[apiKeyHeaderName] = apiKey
  }

  const response = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })

  if (!response.ok) {
    const text = (await response.text()).trim()
    if (response.status === 401) {
      setApiKey(null)
      unauthorizedHandlers.forEach((handler) => handler())
    }
    throw new ApiError(
      response.status,
      text || `${method} ${path} failed with ${response.status}`,
    )
  }

  // 204, and any empty body, decode to undefined rather than blowing up in
  // JSON.parse. DELETE handlers return 202 with a body, but this keeps the
  // client honest if that ever changes.
  const text = await response.text()
  if (!text) {
    return undefined as T
  }
  return JSON.parse(text) as T
}
