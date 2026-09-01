import { useIsFetching, useQueryClient } from '@tanstack/react-query'

import {
  useDirectoryScannerConfig,
  useScanDirectories,
} from '../../api/queries'
import { Button } from '../../components/Card'
import { PageError, PageLoading } from '../../components/PageState'
import { PageHeader } from '../../layout/AppShell'
import { ScanDirectoriesSection } from './ScanDirectoriesSection'
import { ScannerSection } from './ScannerSection'
import { SavingInfoSidebar } from './SavingInfoSidebar'

export function DirectoryScannerPage() {
  const scanner = useDirectoryScannerConfig()
  const directories = useScanDirectories()

  const queryClient = useQueryClient()
  const fetching = useIsFetching({ queryKey: ['config'] })

  const error = scanner.error ?? directories.error

  if (error) {
    return (
      <>
        <PageHeader title="Directory Scanner" />
        <PageError error={error} />
      </>
    )
  }

  if (scanner.isLoading || directories.isLoading) {
    return (
      <>
        <PageHeader title="Directory Scanner" />
        <PageLoading />
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Directory Scanner"
        description="How the background scanner walks the configured libraries, stored in the single application config document. Click a value to edit it."
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
        <ScannerSection parallelCount={scanner.data?.parallelCount ?? 1} />

        <ScanDirectoriesSection directories={directories.data ?? []} />
      </div>
    </>
  )
}

export function DirectoryScannerSidebar() {
  return <SavingInfoSidebar />
}
