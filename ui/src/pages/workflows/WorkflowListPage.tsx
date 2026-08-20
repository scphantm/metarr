import { useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'

import { useWorkflowList } from '../../api/queries'
import { Button, Card, EmptyState } from '../../components/Card'
import { PageHeader } from '../../layout/AppShell'

/*
 * The first infinite-scroll list in this codebase — everywhere else pages a
 * full array from a single useQuery. A sentinel div at the end of the list,
 * watched by an IntersectionObserver, requests the next page as it scrolls
 * into view; kept local to this one component rather than a shared hook,
 * since there's only this one call site so far.
 */
export function WorkflowListPage() {
  const navigate = useNavigate()
  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading, isError, error } =
    useWorkflowList()

  const sentinelRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel || !hasNextPage) return

    const observer = new IntersectionObserver((entries) => {
      if (entries[0].isIntersecting && !isFetchingNextPage) {
        void fetchNextPage()
      }
    })
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  const workflows = data?.pages.flatMap((page) => page.workflows) ?? []

  return (
    <>
      <PageHeader
        title="Workflows"
        description="Visual pipelines built from nodes and edges."
        actions={
          <Button variant="primary" onClick={() => navigate('/workflows/add')}>
            Add Workflow
          </Button>
        }
      />

      <div className="flex flex-col gap-3 px-6 py-5">
        {isError ? (
          <p className="text-sm text-red">
            {error instanceof Error ? error.message : 'Failed to load workflows'}
          </p>
        ) : null}

        {!isLoading && workflows.length === 0 ? (
          <EmptyState>No workflows yet — click Add Workflow to build one.</EmptyState>
        ) : null}

        {workflows.map((workflow) => (
          <Card
            key={workflow.document_id}
            title={workflow.name}
            description={workflow.description}
            actions={
              <Button onClick={() => navigate(`/workflows/${workflow.document_id}/edit`)}>
                Edit
              </Button>
            }
          >
            <div className="flex flex-wrap items-center gap-1.5">
              {workflow.tags.map((tag) => (
                <span
                  key={tag}
                  className="rounded border border-edge-strong/40 bg-surface-hover px-2 py-0.5 text-xs text-ink-strong"
                >
                  {tag}
                </span>
              ))}
              <span className="ml-auto text-xs text-ink-muted">
                v{workflow.version} · {new Date(workflow.created_at).toLocaleString()}
              </span>
            </div>
          </Card>
        ))}

        <div ref={sentinelRef} />
        {isFetchingNextPage ? (
          <p className="py-2 text-center text-xs text-ink-muted">Loading more…</p>
        ) : null}
      </div>
    </>
  )
}
