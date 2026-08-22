# UI Testing

## Overview

The Metarr UI uses **Vitest** for fast, token-efficient unit testing. Tests are designed to catch regressions early while keeping test output concise.

## Running Tests

```bash
# Run all tests once
npm run test

# Run tests in watch mode
npm run test:watch
```

## Test Structure

Tests are colocated with source files in `__tests__` directories:

```
src/
├── lib/
│   ├── utils.ts
│   └── __tests__/
│       ├── utils.test.ts
│       ├── useDebouncedValue.test.ts
│       └── typeIcons.test.ts
└── pages/
    └── workflows/
        ├── connectionRules.ts
        └── __tests__/
            └── connectionRules.test.ts
```

## Test Focus

The initial test suite prioritizes token efficiency by testing:

1. **Pure utility functions** (`cn`, `iconClassForType`, etc.) — fast, deterministic, no side effects
2. **Core business logic** (`isSubtypeOf`, `elementType`, etc.) — validates workflow type system
3. **Critical hooks** (`useDebouncedValue`) — debounce behavior under timer changes

These tests catch regressions in the most frequently-used code paths with minimal test output, making CI runs faster and reports more scannable.

## Coverage

Current test coverage includes:

- **utils.ts** — class name merging and Tailwind handling
- **typeIcons.ts** — icon mapping for workflow types and control ports
- **connectionRules.ts** — type system: list detection, subtype hierarchy, element extraction
- **useDebouncedValue.ts** — debounce timing and value updates

## Writing Tests

### Pattern: Pure Functions

```typescript
describe('isSubtypeOf', () => {
  it('detects dotted prefix hierarchy', () => {
    expect(isSubtypeOf('path.file', 'path')).toBe(true)
  })
})
```

### Pattern: Hooks with Fake Timers

```typescript
describe('useDebouncedValue', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  it('debounces value changes', async () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebouncedValue(value, 100),
      { initialProps: { value: 'first' } }
    )
    
    rerender({ value: 'second' })
    act(() => vi.advanceTimersByTime(100))
    expect(result.current).toBe('second')
  })
})
```

**Key points:**
- Wrap timer advances in `act()` to avoid React warnings
- Use `renderHook` for hook-only tests
- Keep assertions focused on behavior, not implementation

## Token Efficiency

Tests are optimized for token efficiency during runs:

1. **No verbose reporters** — single-line test names
2. **No redundant checks** — assertions test behavior, not every code path
3. **Fast utilities** — pure functions complete in <1ms each
4. **Minimal output** — only failures print details

## Future Expansion

Priority areas for future test coverage:

- **API/query integration** — mocked fetch for catalog, config endpoints
- **React components** — shallow testing of layout and structure (Card, SaveState, etc.)
- **Workflow catalog** — type inference and node registry
- **Router integration** — navigation and page component mounting
- **Error scenarios** — malformed input, network failures
