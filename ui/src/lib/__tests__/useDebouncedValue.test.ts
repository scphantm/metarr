import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useDebouncedValue } from '../useDebouncedValue'

describe('useDebouncedValue', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('returns initial value immediately', () => {
    const { result } = renderHook(() => useDebouncedValue('initial', 100))
    expect(result.current).toBe('initial')
  })

  it('debounces value changes', async () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebouncedValue(value, 100),
      { initialProps: { value: 'first' } }
    )

    expect(result.current).toBe('first')

    rerender({ value: 'second' })
    expect(result.current).toBe('first')

    act(() => vi.advanceTimersByTime(50))
    expect(result.current).toBe('first')

    act(() => vi.advanceTimersByTime(50))
    expect(result.current).toBe('second')
  })

  it('resets debounce timer on rapid changes', async () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebouncedValue(value, 100),
      { initialProps: { value: 'a' } }
    )

    rerender({ value: 'b' })
    act(() => vi.advanceTimersByTime(50))
    rerender({ value: 'c' })
    expect(result.current).toBe('a')

    act(() => vi.advanceTimersByTime(50))
    expect(result.current).toBe('a')

    act(() => vi.advanceTimersByTime(50))
    expect(result.current).toBe('c')
  })

  it('handles number values', async () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebouncedValue(value, 100),
      { initialProps: { value: 1 } }
    )

    expect(result.current).toBe(1)
    rerender({ value: 2 })
    act(() => vi.advanceTimersByTime(100))
    expect(result.current).toBe(2)
  })

  it('respects different delay values', async () => {
    const { result, rerender } = renderHook(
      ({ value, delay }) => useDebouncedValue(value, delay),
      { initialProps: { value: 'first', delay: 200 } }
    )

    rerender({ value: 'second', delay: 200 })
    act(() => vi.advanceTimersByTime(100))
    expect(result.current).toBe('first')

    act(() => vi.advanceTimersByTime(100))
    expect(result.current).toBe('second')
  })

  it('clears timer on unmount', () => {
    const clearSpy = vi.spyOn(window, 'clearTimeout')
    const { unmount, rerender } = renderHook(
      ({ value }) => useDebouncedValue(value, 100),
      { initialProps: { value: 'initial' } }
    )

    rerender({ value: 'changed' })
    unmount()

    expect(clearSpy).toHaveBeenCalled()
    clearSpy.mockRestore()
  })
})
