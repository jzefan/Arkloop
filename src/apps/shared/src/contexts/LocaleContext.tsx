import { createContext, useContext, useState, useMemo, type ReactNode } from 'react'

export type Locale = 'zh' | 'en'

type LocaleContextValue<T> = {
  locale: Locale
  setLocale: (l: Locale) => void
  t: T
}

export function createLocaleContext<T>() {
  const Ctx = createContext<LocaleContextValue<T> | null>(null)

  function LocaleProvider({
    children,
    locales,
    readLocale,
    writeLocale,
  }: {
    children: ReactNode
    locales: Record<Locale, T>
    readLocale: () => Locale
    writeLocale: (l: Locale) => void
  }) {
    const [locale, setLocaleState] = useState<Locale>(readLocale)

    const setLocale = (l: Locale) => {
      writeLocale(l)
      setLocaleState(l)
    }

    const t = useMemo(() => locales[locale], [locales, locale])

    return (
      <Ctx.Provider value={{ locale, setLocale, t }}>
        {children}
      </Ctx.Provider>
    )
  }

  function useLocale(): LocaleContextValue<T> {
    const ctx = useContext(Ctx)
    if (!ctx) throw new Error('useLocale must be used within LocaleProvider')
    return ctx
  }

  return { LocaleProvider, useLocale }
}
