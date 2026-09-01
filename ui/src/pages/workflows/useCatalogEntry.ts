import { useMemo } from 'react'

import type { WorkflowNodeType as NodeType } from '../../gen/metarr/v1/workflow_catalog_pb'
import { useWorkflowCatalog } from '../../api/queries'

/**
 * Looks up one catalog entry. Several entries may share `type` (variations
 * of one plugin, e.g. two core/start entries with different dataOut shapes
 * — they all share one editor component, see nodes/registry.ts), so `id`
 * disambiguates when present; it's absent for legacy node instances saved
 * before catalog entries carried an id, or for types with only one entry
 * that don't bother threading one, in which case this falls back to the
 * first entry matching `type`. Every catalog-driven node component calls
 * this itself rather than having the entry threaded through as a prop, so a
 * catalog refetch reaches every node on the canvas for free through
 * TanStack Query's shared cache.
 */
export function useCatalogEntry(
  id: string | undefined,
  type: string,
): NodeType | undefined {
  const { data } = useWorkflowCatalog()
  return useMemo(() => {
    if (id) {
      const byId = data?.nodeTypes.find((entry) => entry.id === id)
      if (byId) return byId
    }
    return data?.nodeTypes.find((entry) => entry.type === type)
  }, [data, id, type])
}

export function useTransforms() {
  const { data } = useWorkflowCatalog()
  return data?.transforms ?? []
}
