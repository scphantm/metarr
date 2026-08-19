import { useIsFetching, useQueryClient } from '@tanstack/react-query'

import { useSidecarTypes } from '../../api/queries'
import { Button } from '../../components/Card'
import { Spinner } from '../../components/SaveState'
import { PageHeader } from '../../layout/AppShell'
import { SavingInfoSidebar } from './SavingInfoSidebar'
import { SidecarTypesSection } from './SidecarTypesSection'

export function SidecarsPage() {
  const sidecarTypes = useSidecarTypes()

  const queryClient = useQueryClient()
  const fetching = useIsFetching({ queryKey: ['config'] })

  if (sidecarTypes.error) {
    return (
      <>
        <PageHeader title="Sidecars" />
        <div className="px-6 py-5">
          <p className="rounded border border-red/40 bg-red/10 px-4 py-3 text-sm text-red">
            {sidecarTypes.error instanceof Error
              ? sidecarTypes.error.message
              : String(sidecarTypes.error)}
          </p>
        </div>
      </>
    )
  }

  if (sidecarTypes.isLoading) {
    return (
      <>
        <PageHeader title="Sidecars" />
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
        title="Sidecars"
        description="How the scanner classifies non-media files found next to media, stored in the single application config document. Click a value to edit it."
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
        <SidecarTypesSection types={sidecarTypes.data ?? []} />
      </div>
    </>
  )
}

export function SidecarsSidebar() {
  return <SavingInfoSidebar />
}
