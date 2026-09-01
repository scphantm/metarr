import type { ReactNode } from 'react'
import {
  Button as AntButton,
  Card as AntCard,
  Empty,
  Space,
  Typography,
} from 'antd'

import './Card.css'

export function Card({
  title,
  description,
  actions,
  children,
}: {
  title: string
  description?: string
  actions?: ReactNode
  children: ReactNode
}) {
  return (
    <AntCard
      title={
        <div>
          <span className="ui-card-title">{title}</span>
          {description ? (
            <Typography.Text type="secondary" className="ui-card-description">
              {description}
            </Typography.Text>
          ) : null}
        </div>
      }
      extra={actions ? <Space>{actions}</Space> : undefined}
    >
      {children}
    </AntCard>
  )
}

// Row is the label/value pair every edit-in-place field sits in, so labels
// line up down the whole page regardless of which editor a field uses.
export function Row({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="ui-row">
      <div className="ui-row-label-col">
        <div className="ui-row-label">{label}</div>
        {hint ? (
          <Typography.Text type="secondary" className="ui-row-hint">
            {hint}
          </Typography.Text>
        ) : null}
      </div>
      <div className="ui-row-content">{children}</div>
    </div>
  )
}

const variantToButtonProps = {
  default: {},
  primary: { type: 'primary' as const },
  danger: { danger: true },
  ghost: { type: 'text' as const },
}

export function Button({
  children,
  onClick,
  variant = 'default',
  type = 'button',
  disabled,
  title,
}: {
  children: ReactNode
  onClick?: () => void
  variant?: 'default' | 'primary' | 'danger' | 'ghost'
  type?: 'button' | 'submit'
  disabled?: boolean
  title?: string
}) {
  return (
    <AntButton
      htmlType={type}
      onClick={onClick}
      disabled={disabled}
      title={title}
      size="small"
      {...variantToButtonProps[variant]}
    >
      {children}
    </AntButton>
  )
}

export function EmptyState({ children }: { children: ReactNode }) {
  return (
    <Empty
      image={Empty.PRESENTED_IMAGE_SIMPLE}
      description={children}
      className="ui-empty-state"
    />
  )
}
