import { useState } from 'react'
import { NavLink } from 'react-router-dom'

/*
 * The left column. Sections are declared as data so the Searches, Workflows and
 * Automations areas the project is heading for slot in beside System without
 * touching the rendering.
 */

type NavItem = {
  label: string
  to: string
  items?: NavItem[]
}

type NavSection = {
  label: string
  // A section with a route of its own navigates as well as expanding: the
  // chevron toggles the group, the label goes to the page. Sections without
  // one render a plain header that only toggles.
  to?: string
  items: NavItem[]
  // Sections with nothing built yet still appear, so the shape of the app is
  // visible, but they say so rather than offering dead links.
  comingSoon?: boolean
}

const sections: NavSection[] = [
  { label: 'Workflows', to: '/workflows',
    items: [{ label: 'Add Workflow', to: '/workflows/add' }] },
  { label: 'Searches', items: [], comingSoon: true },
  { label: 'Automations', items: [], comingSoon: true },
  { label: 'Tasks', items: [], comingSoon: true },
  {
    label: 'System',
    to: '/system',
    items: [
      {
        label: 'Configuration',
        to: '/system/configuration',
        items: [
          { label: 'Directory Scanner', to: '/system/directory-scanner' },
          { label: 'Sidecars', to: '/system/sidecars' },
          { label: 'Interfaces', to: '/system/interfaces' },
          { label: 'Chatbot', to: '/system/chatbot' },
          { label: 'Security', to: '/system/security' },
        ],
      },
      { label: 'Agents', to: '/system/agents' },
      { label: 'Logging', to: '/system/logging' },
      { label: 'External Tools', to: '/system/external-tools' },
    ],
  },
]

export function NavColumn({
  onNavigate,
  pinned,
  onTogglePin,
}: {
  onNavigate?: () => void
  // Only present on the desktop hover/pin variant (AppShell) — the mobile
  // drawer always shows the full column, so it has no need for a pin control.
  pinned?: boolean
  onTogglePin?: () => void
}) {
  return (
    <nav className="flex h-full flex-col gap-1 p-3" aria-label="Main">
      <div className="flex items-center justify-between px-2 py-3">
        <div className="flex items-center gap-2">
          <span className="text-lg font-semibold tracking-tight text-ink-strong">
            Metarr
          </span>
          <span className="text-xs text-ink-muted">v{__APP_VERSION__}</span>
        </div>
        {onTogglePin ? (
          <button
            type="button"
            onClick={onTogglePin}
            aria-label={pinned ? 'Unpin navigation' : 'Pin navigation open'}
            title={pinned ? 'Unpin' : 'Pin open'}
            className={`shrink-0 transition-colors hover:text-blue ${pinned ? 'text-blue' : 'text-ink-muted'}`}
          >
            <PinIcon pinned={pinned} />
          </button>
        ) : null}
      </div>

      {sections.map((section) => (
        <NavGroup key={section.label} section={section} onNavigate={onNavigate} />
      ))}
    </nav>
  )
}

// A map-pin glyph, filled when pinned and outlined otherwise. Hand-written
// inline SVG rather than a react-icons import: those are banned under src/
// (see eslint.config.js's no-restricted-imports rule) — reserved for the
// generated type-system icons in lib/icons/typeIcons.css.
function PinIcon({ pinned }: { pinned?: boolean }) {
  return (
    <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true">
      <path
        d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7Zm0 9.5A2.5 2.5 0 1 1 12 6a2.5 2.5 0 0 1 0 5.5Z"
        fill={pinned ? 'currentColor' : 'none'}
        stroke="currentColor"
        strokeWidth={pinned ? '0' : '1.4'}
        strokeLinejoin="round"
      />
    </svg>
  )
}

function NavGroup({
  section,
  onNavigate,
}: {
  section: NavSection
  onNavigate?: () => void
}) {
  const [expanded, setExpanded] = useState(!section.comingSoon)
  const disabled = section.comingSoon

  const headerText =
    'text-xs font-semibold tracking-wide uppercase transition-colors'

  return (
    <div>
      {/* The chevron and the label are separate controls so a section that
          has a page of its own can be opened without collapsing the group
          you are looking at. */}
      <div className="flex items-center rounded pr-2">
        <button
          type="button"
          onClick={() => !disabled && setExpanded((current) => !current)}
          aria-expanded={expanded}
          aria-label={expanded ? `Collapse ${section.label}` : `Expand ${section.label}`}
          disabled={disabled}
          className={`flex h-7 w-6 shrink-0 items-center justify-center rounded ${
            disabled
              ? 'cursor-default text-ink-muted/60'
              : 'text-ink-muted hover:bg-surface-hover hover:text-ink-strong'
          }`}
        >
          <span
            aria-hidden="true"
            className={`inline-block transition-transform ${
              expanded && !disabled ? 'rotate-90' : ''
            } ${disabled ? 'opacity-0' : ''}`}
          >
            ›
          </span>
        </button>

        {section.to && !disabled ? (
          <NavLink
            to={section.to}
            onClick={onNavigate}
            end
            className={({ isActive }) =>
              `flex-1 rounded px-1 py-1.5 text-left ${headerText} ${
                isActive
                  ? 'bg-surface-hover text-blue'
                  : 'text-ink-muted hover:bg-surface-hover hover:text-ink-strong'
              }`
            }
          >
            {section.label}
          </NavLink>
        ) : (
          <button
            type="button"
            onClick={() => !disabled && setExpanded((current) => !current)}
            disabled={disabled}
            className={`flex flex-1 items-center rounded px-1 py-1.5 text-left ${headerText} ${
              disabled
                ? 'cursor-default text-ink-muted/60'
                : 'text-ink-muted hover:bg-surface-hover hover:text-ink-strong'
            }`}
          >
            {section.label}
            {disabled ? (
              <span className="ml-auto text-[10px] normal-case opacity-70">
                soon
              </span>
            ) : null}
          </button>
        )}
      </div>

      {expanded && !disabled ? (
        <ul className="mt-0.5 ml-3 flex flex-col gap-0.5 border-l border-edge pl-2">
          {section.items.map((item) => (
            <li key={item.to}>
              <NavLink
                to={item.to}
                onClick={onNavigate}
                className={({ isActive }) =>
                  `block rounded px-2 py-1.5 text-sm transition-colors ${
                    isActive
                      ? 'bg-surface-hover font-medium text-blue'
                      : 'text-ink hover:bg-surface-hover hover:text-ink-strong'
                  }`
                }
              >
                {item.label}
              </NavLink>

              {item.items && item.items.length > 0 ? (
                <ul className="mt-0.5 ml-3 flex flex-col gap-0.5 border-l border-edge pl-2">
                  {item.items.map((subItem) => (
                    <li key={subItem.to}>
                      <NavLink
                        to={subItem.to}
                        onClick={onNavigate}
                        className={({ isActive }) =>
                          `block rounded px-2 py-1.5 text-sm transition-colors ${
                            isActive
                              ? 'bg-surface-hover font-medium text-blue'
                              : 'text-ink hover:bg-surface-hover hover:text-ink-strong'
                          }`
                        }
                      >
                        {subItem.label}
                      </NavLink>
                    </li>
                  ))}
                </ul>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
