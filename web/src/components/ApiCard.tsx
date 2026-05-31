import type { Product } from '../api/types'
import { ApiIcon, categoryTint } from './apiIcons'

export function ApiCard({ p, onSubscribe }: { p: Product; onSubscribe: (p: Product) => void }) {
  return (
    <article className="card in" data-testid="api-card" style={categoryTint(p.category)}>
      <div className="thumb">
        <span className="catbadge">{p.category}</span>
        <span className="ico">
          <ApiIcon name={p.icon} />
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
          <button className="subbtn" onClick={() => onSubscribe(p)}>S'abonner</button>
        </div>
      </div>
    </article>
  )
}
