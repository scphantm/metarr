import { Link } from 'react-router-dom'

import { PageHeader } from '../../layout/AppShell'

const sections = [
  {
    to: '/system/directory-scanner',
    label: 'Directory Scanner',
    description: 'How the background scanner walks the configured libraries.',
  },
  {
    to: '/system/sidecars',
    label: 'Sidecars',
    description: 'How the scanner classifies non-media files found next to media.',
  },
  {
    to: '/system/interfaces',
    label: 'Interfaces',
    description: 'External services Metarr integrates with, like Sonarr.',
  },
  {
    to: '/system/chatbot',
    label: 'Chatbot',
    description: 'The AI provider connected to the chat widget.',
  },
  {
    to: '/system/security',
    label: 'Security',
    description: 'The administrator account and API keys.',
  },
]

export function ConfigurationPage() {
  return (
    <>
      <PageHeader
        title="Configuration"
        description="Configuration settings are grouped into the sections below."
      />

      <div className="flex flex-col gap-3 px-6 py-5">
        {sections.map((section) => (
          <Link
            key={section.to}
            to={section.to}
            className="rounded-lg border border-edge bg-surface px-5 py-4 transition-colors hover:border-edge-strong hover:bg-surface-hover"
          >
            <h2 className="text-sm font-semibold tracking-wide text-ink-strong uppercase">
              {section.label}
            </h2>
            <p className="mt-1 max-w-2xl text-sm text-ink-muted">
              {section.description}
            </p>
          </Link>
        ))}
      </div>
    </>
  )
}
