import { useT } from '../i18n/LanguageProvider'
import type { ViewMode } from './usePersistedView'

/** Two-button segmented control switching between card and list layouts. */
export function ViewToggle({ value, onChange }: { value: ViewMode; onChange: (v: ViewMode) => void }) {
  const t = useT()
  return (
    <div className="viewtoggle" role="group" aria-label={t('catalog.viewMode')}>
      <button type="button" aria-label={t('catalog.gridView')} title={t('catalog.gridView')}
        aria-pressed={value === 'cards'} onClick={() => onChange('cards')}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true">
          <rect x="4" y="4" width="7" height="7" rx="1.5" /><rect x="13" y="4" width="7" height="7" rx="1.5" />
          <rect x="4" y="13" width="7" height="7" rx="1.5" /><rect x="13" y="13" width="7" height="7" rx="1.5" />
        </svg>
      </button>
      <button type="button" aria-label={t('catalog.listView')} title={t('catalog.listView')}
        aria-pressed={value === 'list'} onClick={() => onChange('list')}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true">
          <path d="M4 6h16M4 12h16M4 18h16" strokeLinecap="round" />
        </svg>
      </button>
    </div>
  )
}
