import { Alert, Spin } from 'antd'

// The same "still loading" / "failed to load" shell every config page under
// pages/system renders before it has data to show.

export function PageError({ error }: { error: unknown }) {
  return (
    <div className="page-state-body">
      <Alert type="error" showIcon message={error instanceof Error ? error.message : String(error)} />
    </div>
  )
}

export function PageLoading({ children = 'Loading configuration…' }: { children?: string }) {
  return (
    <div className="page-state-loading">
      <Spin size="small" /> {children}
    </div>
  )
}
