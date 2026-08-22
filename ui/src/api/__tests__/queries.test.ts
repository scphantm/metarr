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
      expect(queryKeys.redisStats).toEqual(['stats', 'redis'])
      expect(queryKeys.agents).toEqual(['stats', 'agents'])
      expect(queryKeys.logTail).toEqual(['stats', 'log-tail'])
    })

    it('defines chatbot keys', () => {
      expect(queryKeys.chatbot).toEqual(['config', 'chatbot'])
      expect(queryKeys.chatSessions).toEqual(['chatbot', 'sessions'])
    })

    it('defines workflow catalog key', () => {
      expect(queryKeys.workflowCatalog).toEqual(['workflows', 'catalog'])
      expect(queryKeys.workflows).toEqual(['workflows'])
    })
  })

  describe('dynamic keys', () => {
    it('generates chat message keys', () => {
      const sessionId = 'session-123'
      expect(queryKeys.chatMessages(sessionId)).toEqual([
        'chatbot',
        'sessions',
        sessionId,
        'messages',
      ])
    })

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
      expect(queryKeys.chatMessages('session-1')).not.toEqual(queryKeys.chatMessages('session-2'))
    })
  })
})
