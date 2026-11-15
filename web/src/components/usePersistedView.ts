import { useCallback, useState } from 'react'

export type ViewMode = 'list' | 'cards'

/**
 * A list/cards layout choice remembered per browser under `key`.
 * Storage is a convenience only: any read/write failure (private mode,
 * blocked storage) silently falls back to `fallback`.
 */
export function usePersistedView(key: string, fallback: ViewMode): [ViewMode, (v: ViewMode) => void] {
  const [view, setView] = useState<ViewMode>(() => {
    try {
      const v = localStorage.getItem(key)
      return v === 'list' || v === 'cards' ? v : fallback
    } catch { return fallback }
  })
  const pick = useCallback((v: ViewMode) => {
    setView(v)
    try { localStorage.setItem(key, v) } catch { /* ignore */ }
  }, [key])
  return [view, pick]
}
