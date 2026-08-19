import type { ReactNode } from 'react'

import { PageHeader } from '../../layout/AppShell'

/*
 * System > External Tools.
 *
 * The stack ships three admin UIs alongside Metarr, plus the API's own
 * Swagger docs. All four are fixed infrastructure — docker-compose port
 * mappings, not application data — so their URLs are built here from the
 * current hostname rather than fetched from an API, the same way the
 * Logging page's "Open in OpenObserve" link already works.
 *
 * Each card links out with a plain new-tab anchor. None of these tools sit
 * behind Metarr's own login; each has its own separate credential, which is
 * why every card says exactly where that credential lives rather than
 * assuming it's obvious.
 */

type Tool = {
  name: string
  href: string
  description: string
  credential: string
  icon: ReactNode
  color: string
}

function tools(hostname: string): Tool[] {
  return [
    {
      name: 'Mongo Express',
      href: `http://${hostname}:6969/`,
      description: 'Browse the MongoDB database directly — collections, documents, indexes.',
      credential:
        'Sign in with MONGO_EXPRESS_USERNAME / MONGO_EXPRESS_PASSWORD from .env (default admin / admin). It arrives already connected to MongoDB.',
      icon: <MongoIcon />,
      color: '#00684A',
    },
    {
      name: 'OpenObserve',
      href: `http://${hostname}:5080/`,
      description: 'Search and filter the centralized log stream from every process.',
      credential:
        'Sign in with OPENOBSERVE_ROOT_EMAIL / OPENOBSERVE_ROOT_PASSWORD from .env.',
      icon: <OpenObserveIcon />,
      color: '#5960F2',
    },
    {
      name: 'Redis Insight',
      href: `http://${hostname}:5540/`,
      description: 'Inspect keys, streams, and Pub/Sub channels on the Redis instance.',
      credential:
        'Pre-connects to Redis automatically using REDIS_PASSWORD from .env — nothing to enter.',
      icon: <RedisIcon />,
      color: '#DC382D',
    },
    {
      name: 'Swagger',
      href: `http://${hostname}:8080/swagger/index.html`,
      description: "The API's own interactive documentation.",
      credential:
        'Click Authorize and paste an API key from System > Security.',
      icon: <SwaggerIcon />,
      color: '#85C742',
    },
  ]
}

export function ExternalToolsPage() {
  const cards = tools(window.location.hostname)

  return (
    <>
      <PageHeader
        title="External Tools"
        description="The infrastructure UIs behind Metarr. Each has its own login, separate from your Metarr account."
      />

      <div className="grid grid-cols-1 gap-4 px-6 py-5 sm:grid-cols-2">
        {cards.map((tool) => (
          <a
            key={tool.name}
            href={tool.href}
            target="_blank"
            rel="noreferrer"
            className="flex flex-col gap-3 rounded-lg border border-edge bg-surface px-5 py-4 transition-colors hover:border-edge-strong hover:bg-surface-hover"
          >
            <div className="flex items-center gap-3">
              <span
                className="flex h-8 w-8 shrink-0 items-center justify-center rounded"
                style={{ color: tool.color }}
                aria-hidden="true"
              >
                {tool.icon}
              </span>
              <div className="min-w-0">
                <h2 className="text-sm font-semibold tracking-wide text-ink-strong uppercase">
                  {tool.name}
                </h2>
                <p className="truncate font-mono text-xs text-ink-muted">
                  {tool.href}
                </p>
              </div>
            </div>

            <p className="text-sm text-ink-muted">{tool.description}</p>

            <p className="rounded border border-edge/60 bg-canvas px-3 py-2 text-xs leading-relaxed text-ink-muted">
              {tool.credential}
            </p>
          </a>
        ))}
      </div>
    </>
  )
}

// Small brand-colored marks rather than fetched logo assets — recognizable at
// a glance without pulling in external image files. currentColor picks up
// each card's brand color from the wrapping span.

function MongoIcon() {
  return (
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" aria-hidden="true">
      <path
        d="M12 2c2.5 3 4 6.5 4 10.5 0 4-1.8 7-4 9.5-2.2-2.5-4-5.5-4-9.5C8 8.5 9.5 5 12 2Z"
        fill="currentColor"
      />
      <path d="M12 15v7" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  )
}

function RedisIcon() {
  return (
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" aria-hidden="true">
      <ellipse cx="12" cy="6" rx="8" ry="3" fill="currentColor" />
      <path
        d="M4 6v5c0 1.66 3.58 3 8 3s8-1.34 8-3V6"
        stroke="currentColor"
        strokeWidth="1.6"
        fill="none"
      />
      <path
        d="M4 11v5c0 1.66 3.58 3 8 3s8-1.34 8-3v-5"
        stroke="currentColor"
        strokeWidth="1.6"
        fill="none"
      />
    </svg>
  )
}

function OpenObserveIcon() {
  return (
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" aria-hidden="true">
      <rect
        x="4"
        y="4"
        width="16"
        height="16"
        rx="4"
        transform="rotate(45 12 12)"
        fill="currentColor"
      />
    </svg>
  )
}

function SwaggerIcon() {
  return (
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" aria-hidden="true">
      <path
        d="M8 4c-2 0-3 1-3 3v2c0 1.5-1 2-2 2 1 0 2 .5 2 2v2c0 2 1 3 3 3M16 4c2 0 3 1 3 3v2c0 1.5 1 2 2 2-1 0-2 .5-2 2v2c0 2-1 3-3 3"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinecap="round"
        fill="none"
      />
    </svg>
  )
}

export function ExternalToolsSidebar() {
  return (
    <section>
      <h2 className="mb-2 text-xs font-semibold tracking-wide text-ink-muted uppercase">
        About these tools
      </h2>
      <div className="rounded border border-edge bg-surface px-3 py-2.5 text-xs leading-relaxed text-ink-muted">
        <p>
          Each panel links to infrastructure Metarr runs alongside itself —
          the database, the cache, the log pipeline, and the API's own docs.
          None of them check your Metarr login; each has its own credential,
          noted on its card.
        </p>
        <p className="mt-2">
          These addresses assume the tool is reachable on the same host you're
          viewing Metarr from, at the port docker-compose publishes it on.
        </p>
      </div>
    </section>
  )
}
