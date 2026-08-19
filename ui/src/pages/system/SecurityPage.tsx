import { useIsFetching, useQueryClient } from '@tanstack/react-query'

import { useConfig } from '../../api/queries'
import { Button } from '../../components/Card'
import { Spinner } from '../../components/SaveState'
import { PageHeader } from '../../layout/AppShell'
import { AdminSection } from './AdminSection'
import { ApiKeysSection } from './ApiKeysSection'
import { SavingInfoSidebar } from './SavingInfoSidebar'

export function SecurityPage() {
  const config = useConfig()

  const queryClient = useQueryClient()
  const fetching = useIsFetching({ queryKey: ['config'] })

  if (config.error) {
    return (
      <>
        <PageHeader title="Security" />
        <div className="px-6 py-5">
          <p className="rounded border border-red/40 bg-red/10 px-4 py-3 text-sm text-red">
            {config.error instanceof Error
              ? config.error.message
              : String(config.error)}
          </p>
        </div>
      </>
    )
  }

  if (config.isLoading || !config.data) {
    return (
      <>
        <PageHeader title="Security" />
        <div className="flex items-center gap-2 px-6 py-5 text-sm text-ink-muted">
          <Spinner />
          Loading configuration…
        </div>
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Security"
        description="The administrator account and API keys, both stored in the single application config document. Click a value to edit it."
        actions={
          <Button
            onClick={() =>
              void queryClient.invalidateQueries({ queryKey: ['config'] })
            }
            title="Re-read every configuration section from the server"
          >
            {fetching ? 'Refreshing…' : 'Refresh'}
          </Button>
        }
      />

      <div className="flex flex-col gap-5 px-6 py-5">
        <AdminSection admin={config.data.admin} />

        <ApiKeysSection config={config.data} />
      </div>
    </>
  )
}

export function SecuritySidebar() {
  return <SavingInfoSidebar />
}
