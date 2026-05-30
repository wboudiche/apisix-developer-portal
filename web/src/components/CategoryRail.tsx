export function CategoryRail({
  categories, active, onPick,
}: { categories: { name: string; count: number }[]; active: string | null; onPick: (c: string | null) => void }) {
  return (
    <aside className="rail">
      <div className="rail-head"><h2>Catégories d'API</h2></div>
      <nav>
        <button className={`cat ${active === null ? 'active' : ''}`} onClick={() => onPick(null)}>
          <span className="clabel">Toutes les catégories</span>
          <span className="cnt">{categories.reduce((n, c) => n + c.count, 0)}</span>
        </button>
        {categories.map(c => (
          <button key={c.name} className={`cat ${active === c.name ? 'active' : ''}`} onClick={() => onPick(c.name)}>
            <span className="clabel">{c.name}</span><span className="cnt">{c.count}</span>
          </button>
        ))}
      </nav>
    </aside>
  )
}
