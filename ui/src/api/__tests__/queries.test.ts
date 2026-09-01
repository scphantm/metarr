import { describe, it, expect } from 'vitest'
import { queryKeys } from '../queries'

describe('queryKeys', () => {
  describe('static keys', () => {
    it('defines config key', () => {
      expect(queryKeys.config).toEqual(['config'])
    })

    it('defines nested config keys', () => {
      expect(queryKeys.directoryScanner).toEqual(['config', 'directory-scanner'])
      expect(queryKeys.scanDirectories).toEqual(['config', 'scan-directories'])
      expect(queryKeys.sidecarTypes).toEqual(['config', 'sidecar-types'])
    })

    it('defines stats keys', () => {
      expect(queryKeys.busSnapshot).toEqual(['stats', 'bus'])
      expect(queryKeys.agents).toEqual(['stats', 'agents'])
      expect(queryKeys.logTail).toEqual(['stats', 'log-tail'])
    })

    it('defines workflow catalog key', () => {
      expect(queryKeys.workflowCatalog).toEqual(['workflows', 'catalog'])
      expect(queryKeys.workflows).toEqual(['workflows'])
    })
  })

  describe('dynamic keys', () => {
    it('generates workflow keys by id', () => {
      const workflowId = 'workflow-456'
      expect(queryKeys.workflow(workflowId)).toEqual(['workflows', workflowId])
      expect(queryKeys.workflowVersions(workflowId)).toEqual([
        'workflows',
        workflowId,
        'versions',
      ])
    })

    it('handles different ids independently', () => {
      expect(queryKeys.workflow('id-a')).not.toEqual(queryKeys.workflow('id-b'))
    })
  })
})
