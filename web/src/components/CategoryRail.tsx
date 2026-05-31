import { categoryDotColor } from './apiIcons'

export function CategoryRail({
  categories, active, onPick,
  tags = [], activeTag = null, onPickTag = () => {},
}: {
  categories: { name: string; count: number }[]
  active: string | null
  onPick: (c: string | null) => void
  tags?: string[]
  activeTag?: string | null
  onPickTag?: (t: string | null) => void
}) {
  const total = categories.reduce((n, c) => n + c.count, 0)
  return (
    <aside className="rail">
      <div className="rail-head">
        <h2>Catégories d'API</h2>
        <button className="collapse" aria-label="Fermer">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
            <path d="M15 6l-6 6 6 6" strokeLinecap="round" strokeLinejoin="round"/>
          </svg>
        </button>
      </div>
      <nav>
        <button className={`cat ${active === null ? 'active' : ''}`} onClick={() => onPick(null)}>
          <span className="dot" style={{ background: 'var(--accent)' }} />
          <span className="clabel">Toutes les catégories</span>
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
          <div className="rail-sec">Tags</div>
          <div className="tags">
            {tags.map(t => (
              <button key={t} className={`tag ${activeTag === t ? 'active' : ''}`} onClick={() => onPickTag(activeTag === t ? null : t)}>{t}</button>
            ))}
          </div>
        </>
      )}
      <p className="rail-note">{total} APIs publiées · sandbox &amp; production disponibles. Abonnez-vous pour générer vos clés.</p>
    </aside>
  )
}
