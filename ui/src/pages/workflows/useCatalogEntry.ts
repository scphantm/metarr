import { useMemo } from 'react'

import { useWorkflowCatalog } from '../../api/queries'
import type { NodeType } from './catalogTypes'

/**
 * Looks up one catalog entry by its `type` (all typeVersions of a type
 * share one editor component for now, so version isn't part of the key
 * here — see nodes/registry.ts). Every catalog-driven node component calls
 * this itself rather than having the type threaded through as a prop, so a
 * catalog refetch reaches every node on the canvas for free through
 * TanStack Query's shared cache.
 */
export function useCatalogEntry(type: string): NodeType | undefined {
  const { data } = useWorkflowCatalog()
  return useMemo(() => data?.node_types.find((entry) => entry.type === type), [data, type])
}

export function useTransforms() {
  const { data } = useWorkflowCatalog()
  return data?.transforms ?? []
}
