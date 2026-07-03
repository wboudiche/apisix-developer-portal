import { categoryDotColor } from './apiIcons'
import { useT } from '../i18n/LanguageProvider'

export function CategoryRail({
  categories, active, onPick,
  tags = [], activeTag = null, onPickTag = () => {},
  open = true, onClose = () => {},
}: {
  categories: { name: string; count: number }[]
  active: string | null
  onPick: (c: string | null) => void
  tags?: string[]
  activeTag?: string | null
  onPickTag?: (t: string | null) => void
  open?: boolean
  onClose?: () => void
}) {
  const t = useT()
  const total = categories.reduce((n, c) => n + c.count, 0)
  return (
    <aside className={`rail ${open ? 'open' : 'closed'}`}>
      <div className="rail-head">
        <h2>{t('categoryRail.title')}</h2>
        <button className="collapse" aria-label={t('common.close')} onClick={onClose}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
            <path d="M15 6l-6 6 6 6" strokeLinecap="round" strokeLinejoin="round"/>
          </svg>
        </button>
      </div>
      <nav>
        <button className={`cat ${active === null ? 'active' : ''}`} onClick={() => onPick(null)}>
          <span className="dot" style={{ background: 'var(--accent)' }} />
          <span className="clabel">{t('categoryRail.allCategories')}</span>
          <span className="cnt">{total}</span>
        </button>
        {categories.map(c => (
          <button key={c.name} className={`cat ${active === c.name ? 'active' : ''}`} onClick={() => onPick(c.name)}>
            <span className="dot" style={{ background: categoryDotColor(c.name) }} />
            <span className="clabel">{c.name}</span><span className="cnt">{c.count}</span>
          </button>
        ))}
      </nav>
      {tags.length > 0 && (
        <>
          <div className="rail-sec">{t('categoryRail.tagsHeading')}</div>
          <div className="tags">
            {tags.map(tg => (
              <button key={tg} className={`tag ${activeTag === tg ? 'active' : ''}`} onClick={() => onPickTag(activeTag === tg ? null : tg)}>{tg}</button>
            ))}
          </div>
        </>
      )}
      <p className="rail-note">{t('categoryRail.note', { total })}</p>
    </aside>
  )
}
