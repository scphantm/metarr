/*
 * API key storage and the stale-session notification path, shared by every
 * transport. The REST-era request() helper is gone — every domain now goes
 * through transport.ts's gRPC-Web transport, and the UI makes no REST calls
 * of its own (the server's only remaining REST route, GET /api/heartbeat,
 * isn't called from the frontend).
 */

const sessionStorageKey = 'metarr.apiKey'

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
// session — any Code.Unauthenticated response, not just the next explicit
// auth call.
const unauthorizedHandlers = new Set<() => void>()

export function onUnauthorized(handler: () => void): () => void {
  unauthorizedHandlers.add(handler)
  return () => unauthorizedHandlers.delete(handler)
}

// Fires every registered onUnauthorized handler. transport.ts's Connect
// interceptor calls this on a Code.Unauthenticated error (see
// AuthContext.tsx's onUnauthorized(clearSession)).
export function notifyUnauthorized(): void {
  unauthorizedHandlers.forEach((handler) => handler())
}
