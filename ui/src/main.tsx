import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter } from 'react-router-dom'
import { Code, ConnectError } from '@connectrpc/connect'

import { App } from './App'
import { AuthProvider } from './auth/AuthContext'
import { ThemeProvider } from './theme/ThemeContext'
import './index.css'
import './lib/icons/typeIcons.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Config changes rarely and every mutation invalidates explicitly, so a
      // short stale time avoids refetch storms while a save is settling.
      staleTime: 5_000,
      refetchOnWindowFocus: true,
      retry: (failureCount, error) => {
        // Retrying an auth failure just burns requests on a key that will not
        // start working; everything else gets two more attempts.
        if (
          error instanceof ConnectError &&
          (error.code === Code.Unauthenticated ||
            error.code === Code.PermissionDenied)
        ) {
          return false
        }
        return failureCount < 2
      },
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <AuthProvider>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </AuthProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
)
