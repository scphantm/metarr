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

export function NavColumn({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <nav className="flex h-full flex-col gap-1 p-3" aria-label="Main">
      <div className="px-2 py-3">
        <span className="text-lg font-semibold tracking-tight text-ink-strong">
          Metarr
        </span>
      </div>

      {sections.map((section) => (
        <NavGroup key={section.label} section={section} onNavigate={onNavigate} />
      ))}
    </nav>
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
