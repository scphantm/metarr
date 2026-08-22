import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

afterEach(() => {
  cleanup()
})

// Silence unhandled promise rejections in tests
process.on('unhandledRejection', () => {})
