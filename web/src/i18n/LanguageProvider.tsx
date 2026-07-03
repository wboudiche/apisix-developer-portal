import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { translate, type Lang } from './t'

function detect(): Lang {
  const s = localStorage.getItem('lang')
  if (s === 'fr' || s === 'en') return s
  return navigator.language?.toLowerCase().startsWith('en') ? 'en' : 'fr'
}

type TFunc = (key: string, vars?: Record<string, string | number>) => string
const Ctx = createContext<{ lang: Lang; setLang: (l: Lang) => void; t: TFunc }>({ lang: 'fr', setLang: () => {}, t: (k) => k })

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [lang, setLang] = useState<Lang>(detect)
  useEffect(() => {
    document.documentElement.setAttribute('lang', lang)
    localStorage.setItem('lang', lang)
  }, [lang])
  const t = useMemo<TFunc>(() => (key, vars) => translate(lang, key, vars), [lang])
  return <Ctx.Provider value={{ lang, setLang, t }}>{children}</Ctx.Provider>
}

export const useLang = () => { const { lang, setLang } = useContext(Ctx); return { lang, setLang } }
export const useT = () => useContext(Ctx).t
