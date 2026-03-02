import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

export type Theme = 'system' | 'light' | 'dark'

type ThemeContextValue = {
  theme: Theme
  setTheme: (t: Theme) => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

export function ThemeProvider({
  children,
  readTheme,
  writeTheme,
}: {
  children: ReactNode
  readTheme: () => Theme
  writeTheme: (t: Theme) => void
}) {
  const [theme, setThemeState] = useState<Theme>(readTheme)

  useEffect(() => {
    const root = document.documentElement
    if (theme === 'system') {
      root.removeAttribute('data-theme')
    } else {
      root.setAttribute('data-theme', theme)
    }
  }, [theme])

  const setTheme = (t: Theme) => {
    writeTheme(t)
    setThemeState(t)
  }

  return (
    <ThemeContext.Provider value={{ theme, setTheme }}>
      {children}
    </ThemeContext.Provider>
  )
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider')
  return ctx
}
