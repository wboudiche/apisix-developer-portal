import type { Product } from '../api/types'

const CAT_DOT: Record<string, string> = {
  Administration: 'var(--c-admin)', Finance: 'var(--c-finance)',
  Marketing: 'var(--c-marketing)', Engineering: 'var(--c-eng)',
}

export function ApiCard({ p }: { p: Product }) {
  return (
    <article className="card in" data-testid="api-card">
      <div className="thumb">
        <span className="catbadge">{p.category}</span>
        <span className="ico" style={{ background: `linear-gradient(150deg, ${CAT_DOT[p.category] ?? 'var(--accent)'}, var(--accent-d))` }}>
          {p.icon.slice(0, 2).toUpperCase()}
        </span>
      </div>
      <div className="cbody">
        <div className="crow1"><span className="cname">{p.name}</span></div>
        <p className="cdesc">{p.description}</p>
        <div className="cmeta">
          <span className="pill">v<b>{p.version}</b></span>
          <span className="pill ctx">{p.contextPath}</span>
        </div>
        <div className="cfoot">
          <div className="ctags">{p.tags.slice(0, 2).map(t => <span key={t} className="ctag">{t}</span>)}</div>
          <button className="subbtn">S'abonner</button>
        </div>
      </div>
    </article>
  )
}
