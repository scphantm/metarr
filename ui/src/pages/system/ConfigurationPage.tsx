import { Link } from 'react-router-dom'
import { Card, Typography } from 'antd'

import { PageHeader } from '../../layout/AppShell'
import './ConfigurationPage.css'

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

      <div className="page-body">
        {sections.map((section) => (
          <Link key={section.to} to={section.to} className="configuration-section-link">
            <Card hoverable size="small">
              <Typography.Text className="configuration-section-label">
                {section.label}
              </Typography.Text>
              <Typography.Text type="secondary" className="configuration-section-description">
                {section.description}
              </Typography.Text>
            </Card>
          </Link>
        ))}
      </div>
    </>
  )
}
