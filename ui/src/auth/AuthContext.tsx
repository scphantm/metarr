import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { useQueryClient } from '@tanstack/react-query'

import { getApiKey, onUnauthorized, request, setApiKey } from '../api/client'
import { resetSocket } from '../api/socket'
import type { LoginRequest, LoginResponse } from '../api/types'

type AuthContextValue = {
  isAuthenticated: boolean
  username: string | null
  expiresAt: number | null
  login: (credentials: LoginRequest) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

const usernameKey = 'metarr.username'
const expiresAtKey = 'metarr.expiresAt'

function readStored(key: string): string | null {
  try {
    return sessionStorage.getItem(key)
  } catch {
    return null
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [isAuthenticated, setIsAuthenticated] = useState(() => Boolean(getApiKey()))
  const [username, setUsername] = useState<string | null>(() =>
    readStored(usernameKey),
  )
  const [expiresAt, setExpiresAt] = useState<number | null>(() => {
    const stored = readStored(expiresAtKey)
    return stored ? Number(stored) : null
  })

  const clearSession = useCallback(() => {
    // The socket authenticated with the key being discarded, so it has to go
    // too — otherwise it keeps streaming as the previous user, and reconnects
    // with a key that is no longer valid.
    resetSocket()
    setApiKey(null)
    setIsAuthenticated(false)
    setUsername(null)
    setExpiresAt(null)
    try {
      sessionStorage.removeItem(usernameKey)
      sessionStorage.removeItem(expiresAtKey)
    } catch {
      // Nothing to clear if storage was never available.
    }
    queryClient.clear()
  }, [queryClient])

  // A key can go stale mid-session — the server's sessions expire on their own
  // schedule. Any 401 from any request drops us back to the login screen rather
  // than leaving a shell that silently fails every read.
  useEffect(() => onUnauthorized(clearSession), [clearSession])

  const login = useCallback(
    async (credentials: LoginRequest) => {
      const response = await request<LoginResponse>('/api/auth/login', {
        method: 'POST',
        body: credentials,
        anonymous: true,
      })

      setApiKey(response.api_key)
      const expiry = Date.now() + response.expires_in_seconds * 1000
      setIsAuthenticated(true)
      setUsername(credentials.username)
      setExpiresAt(expiry)
      try {
        sessionStorage.setItem(usernameKey, credentials.username)
        sessionStorage.setItem(expiresAtKey, String(expiry))
      } catch {
        // Session still works; only the reload-survival is lost.
      }
    },
    [],
  )

  const logout = useCallback(async () => {
    try {
      await request('/api/auth/logout', { method: 'POST' })
    } catch {
      // The key is being discarded either way — a failed revoke should not
      // strand someone in a session they asked to leave.
    }
    clearSession()
  }, [clearSession])

  const value = useMemo(
    () => ({ isAuthenticated, username, expiresAt, login, logout }),
    [isAuthenticated, username, expiresAt, login, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext)
  if (!context) {
    throw new Error('useAuth must be used inside an AuthProvider')
  }
  return context
}
