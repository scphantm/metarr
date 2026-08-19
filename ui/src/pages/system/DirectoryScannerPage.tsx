import { useIsFetching, useQueryClient } from '@tanstack/react-query'

import { useDirectoryScannerConfig, useScanDirectories } from '../../api/queries'
import { Button } from '../../components/Card'
import { Spinner } from '../../components/SaveState'
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
        <div className="px-6 py-5">
          <p className="rounded border border-red/40 bg-red/10 px-4 py-3 text-sm text-red">
            {error instanceof Error ? error.message : String(error)}
          </p>
        </div>
      </>
    )
  }

  if (scanner.isLoading || directories.isLoading) {
    return (
      <>
        <PageHeader title="Directory Scanner" />
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

      <div className="flex flex-col gap-5 px-6 py-5">
        <ScannerSection parallelCount={scanner.data?.parallel_count ?? 1} />

        <ScanDirectoriesSection directories={directories.data ?? []} />
      </div>
    </>
  )
}

export function DirectoryScannerSidebar() {
  return <SavingInfoSidebar />
}
