import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

afterEach(() => {
  cleanup()
})

// jsdom ships no matchMedia; antd's responsive grid (Row/Col) calls it on
// mount. A stub that always reports "no match" is enough for component tests.
if (typeof window !== 'undefined' && !window.matchMedia) {
  window.matchMedia = (query: string): MediaQueryList =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }) as MediaQueryList
}

// jsdom's getComputedStyle throws on the pseudo-element form, which antd's
// table calls through rc-util's scrollbar measurement on mount. Drop the
// pseudo-element argument so the call resolves to a real style object.
if (typeof window !== 'undefined') {
  const nativeGetComputedStyle = window.getComputedStyle.bind(window)
  window.getComputedStyle = ((element: Element) =>
    nativeGetComputedStyle(element)) as typeof window.getComputedStyle
}

// Silence unhandled promise rejections in tests
process.on('unhandledRejection', () => {})
