import { useIsFetching, useQueryClient } from '@tanstack/react-query'

import { useSonarrInstances } from '../../api/queries'
import { Button } from '../../components/Card'
import { PageError, PageLoading } from '../../components/PageState'
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
        <PageError error={sonarr.error} />
      </>
    )
  }

  if (sonarr.isLoading) {
    return (
      <>
        <PageHeader title="Interfaces" />
        <PageLoading />
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

      <div className="page-body">
        <SonarrSection instances={sonarr.data ?? []} />
      </div>
    </>
  )
}

export function InterfacesSidebar() {
  return <SavingInfoSidebar />
}
