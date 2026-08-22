import { describe, it, expect } from 'vitest'
import { cn } from '../utils'

describe('cn', () => {
  it('combines class names', () => {
    expect(cn('px-2', 'py-1')).toBe('px-2 py-1')
  })

  it('handles undefined and null', () => {
    expect(cn('px-2', undefined, 'py-1', null)).toBe('px-2 py-1')
  })

  it('merges tailwind classes', () => {
    expect(cn('px-2 px-4')).toBe('px-4')
  })

  it('respects order precedence', () => {
    expect(cn('px-2', 'px-4')).toBe('px-4')
  })

  it('handles object syntax', () => {
    expect(cn({ 'px-2': true, 'py-1': false })).toBe('px-2')
  })

  it('handles arrays', () => {
    expect(cn(['px-2', 'py-1'])).toBe('px-2 py-1')
  })

  it('empty input returns empty string', () => {
    expect(cn()).toBe('')
  })
})
