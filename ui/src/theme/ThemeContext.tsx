import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { App as AntdApp, ConfigProvider } from 'antd'

import { antdThemeFor } from './antdTheme'

export type Theme = 'dark' | 'light'

const storageKey = 'metarr.theme'

type ThemeContextValue = {
  theme: Theme
  setTheme: (theme: Theme) => void
  toggleTheme: () => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

// storedTheme reads the persisted choice. Dark is the default: it is what the
// index.html bootstrap assumes, and the two have to agree or the page flashes.
function storedTheme(): Theme {
  try {
    return localStorage.getItem(storageKey) === 'light' ? 'light' : 'dark'
  } catch {
    return 'dark'
  }
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(storedTheme)

  useEffect(() => {
    document.documentElement.className = `theme-${theme}`
    try {
      localStorage.setItem(storageKey, theme)
    } catch {
      // A browser refusing storage still gets a working theme for this
      // session; only the persistence is lost.
    }
  }, [theme])

  const setTheme = useCallback((next: Theme) => setThemeState(next), [])
  const toggleTheme = useCallback(
    () => setThemeState((current) => (current === 'dark' ? 'light' : 'dark')),
    [],
  )

  const value = useMemo(
    () => ({ theme, setTheme, toggleTheme }),
    [theme, setTheme, toggleTheme],
  )

  return (
    <ThemeContext.Provider value={value}>
      <ConfigProvider theme={antdThemeFor(theme)}>
        <AntdApp>{children}</AntdApp>
      </ConfigProvider>
    </ThemeContext.Provider>
  )
}

export function useTheme(): ThemeContextValue {
  const context = useContext(ThemeContext)
  if (!context) {
    throw new Error('useTheme must be used inside a ThemeProvider')
  }
  return context
}
