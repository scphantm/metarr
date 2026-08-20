import { useState } from 'react'

import { useChatbotConfig } from '../api/queries'
import type { ChatbotConfig } from '../api/types'
import { ChatIcon } from './ChatIcon'
import { ChatPanel } from './ChatPanel'

// True only when the active provider actually has enough to make a call —
// enabled alone isn't enough, since a freshly-enabled section with no API
// key (or, for MCP, no transport target) would just fail on first send.
function isProviderConfigured(config: ChatbotConfig): boolean {
  switch (config.provider) {
    case 'claude':
      return config.claude.api_key.trim() !== ''
    case 'openai':
      return config.openai.api_key.trim() !== ''
    case 'gemini':
      return config.gemini.api_key.trim() !== ''
    case 'mcp': {
      const hasTarget =
        config.mcp.transport === 'stdio' ? Boolean(config.mcp.command?.trim()) : Boolean(config.mcp.url?.trim())
      return Boolean(config.mcp.tool_name.trim()) && hasTarget
    }
    default:
      return false
  }
}

/*
 * Docked in the lower-left corner, present on every page — mounted as a
 * sibling of <Routes> in App.tsx, above every <AppShell> instance, since
 * each route independently wraps its page in its own AppShell (there is no
 * single wrapper above <Routes> to hang persistent chrome off instead). A
 * plain fixed-position div, not a portal: it's already rendered at the app
 * root, above everything including AppShell's own z-40 mobile nav drawer.
 *
 * Renders nothing at all — not even the toggle — until a provider is both
 * enabled and actually configured, so switching it on in System > Chatbot
 * is what makes the icon appear (the config mutation there already
 * invalidates this same query, so no reload is needed).
 */
export function ChatWidget() {
  const [open, setOpen] = useState(false)
  const config = useChatbotConfig()

  if (!config.data || !config.data.enabled || !isProviderConfigured(config.data)) {
    return null
  }

  return (
    <div className="fixed bottom-4 left-4 z-50 flex flex-col items-start gap-2">
      {open ? (
        <div className="flex h-[32rem] w-96 flex-col overflow-hidden rounded-lg border border-edge bg-surface shadow-lg">
          <div className="flex items-center justify-between border-b border-edge px-3 py-2">
            <span className="text-sm font-semibold text-ink-strong">Chat</span>
            <button
              type="button"
              onClick={() => setOpen(false)}
              aria-label="Close chat"
              className="text-ink-muted hover:text-ink-strong"
            >
              ✕
            </button>
          </div>
          <div className="min-h-0 flex-1">
            <ChatPanel />
          </div>
        </div>
      ) : null}

      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        aria-label={open ? 'Close chat' : 'Open chat'}
        aria-expanded={open}
        className="flex h-12 w-12 items-center justify-center rounded-full border border-edge bg-blue text-canvas shadow-lg transition-opacity hover:opacity-90"
      >
        <ChatIcon />
      </button>
    </div>
  )
}
