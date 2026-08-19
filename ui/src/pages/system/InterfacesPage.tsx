import { useIsFetching, useQueryClient } from '@tanstack/react-query'

import { useSonarrInstances } from '../../api/queries'
import { Button } from '../../components/Card'
import { Spinner } from '../../components/SaveState'
import { PageHeader } from '../../layout/AppShell'
import { SavingInfoSidebar } from './SavingInfoSidebar'
import { SonarrSection } from './SonarrSection'

export function InterfacesPage() {
  const sonarr = useSonarrInstances()

  const queryClient = useQueryClient()
  const fetching = useIsFetching({ queryKey: ['config'] })

  if (sonarr.error) {
    return (
      <>
        <PageHeader title="Interfaces" />
        <div className="px-6 py-5">
          <p className="rounded border border-red/40 bg-red/10 px-4 py-3 text-sm text-red">
            {sonarr.error instanceof Error
              ? sonarr.error.message
              : String(sonarr.error)}
          </p>
        </div>
      </>
    )
  }

  if (sonarr.isLoading) {
    return (
      <>
        <PageHeader title="Interfaces" />
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
        title="Interfaces"
        description="External services Metarr integrates with, stored in the single application config document. Click a value to edit it."
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
        <SonarrSection instances={sonarr.data ?? []} />
      </div>
    </>
  )
}

export function InterfacesSidebar() {
  return <SavingInfoSidebar />
}
