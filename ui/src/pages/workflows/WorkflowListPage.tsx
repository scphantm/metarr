import { useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { Alert, Space, Spin, Tag, Typography } from 'antd'
import { timestampDate } from '@bufbuild/protobuf/wkt'

import { useWorkflowList } from '../../api/queries'
import { Button, Card, EmptyState } from '../../components/Card'
import { PageHeader } from '../../layout/AppShell'
import './WorkflowListPage.css'

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

      <div className="page-body">
        {isError ? (
          <Alert
            type="error"
            showIcon
            message={error instanceof Error ? error.message : 'Failed to load workflows'}
          />
        ) : null}

        {!isLoading && workflows.length === 0 ? (
          <EmptyState>No workflows yet — click Add Workflow to build one.</EmptyState>
        ) : null}

        {workflows.map((workflow) => (
          <Card
            key={workflow.documentId}
            title={workflow.name}
            description={workflow.description}
            actions={
              <Button onClick={() => navigate(`/workflows/${workflow.documentId}/edit`)}>
                Edit
              </Button>
            }
          >
            <div className="workflow-list-tags-row">
              <Space size={4} wrap>
                {workflow.tags.map((tag) => (
                  <Tag key={tag}>{tag}</Tag>
                ))}
              </Space>
              <Typography.Text type="secondary" className="workflow-list-meta">
                v{workflow.version} · {workflow.createdAt ? timestampDate(workflow.createdAt).toLocaleString() : ''}
              </Typography.Text>
            </div>
          </Card>
        ))}

        <div ref={sentinelRef} />
        {isFetchingNextPage ? (
          <div className="workflow-list-loading-more">
            <Spin size="small" /> Loading more…
          </div>
        ) : null}
      </div>
    </>
  )
}
