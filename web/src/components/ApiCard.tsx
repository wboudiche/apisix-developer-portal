import { Link } from 'react-router-dom'
import type { Product } from '../api/types'
import { ApiIcon, categoryTint } from './apiIcons'

function Stars({ rating }: { rating: number }) {
  return (
    <span className="stars" title={`${rating}/5`}>
      <svg width="0" height="0" style={{ position: 'absolute' }} aria-hidden="true">
        <defs>
          <linearGradient id="star-half">
            <stop offset="50%" stopColor="currentColor" />
            <stop offset="50%" stopColor="transparent" />
          </linearGradient>
        </defs>
      </svg>
      {[1, 2, 3, 4, 5].map(i => {
        const isFull = rating >= i
        const isHalf = !isFull && rating >= i - 0.5
        const fill = isFull ? 'currentColor' : isHalf ? 'url(#star-half)' : 'none'
        const cls = isFull || isHalf ? 'star-f' : 'star-e'
        return (
          <svg key={i} viewBox="0 0 24 24" className={cls} fill={fill} stroke="currentColor" strokeWidth={1.4}>
            <path d="M12 3.6l2.5 5 5.5.8-4 3.9.95 5.5L12 16.2 7.05 18.7 8 13.2l-4-3.9 5.5-.8z"/>
          </svg>
        )
      })}
    </span>
  )
}

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
        <div className="crow1">
          <Link className="cname" to={`/catalog/${p.slug}`}>{p.name}</Link>
          {p.ratingCount > 0
            ? <span className="ratewrap"><Stars rating={p.rating} /> <span className="ratecount">({p.ratingCount})</span></span>
            : <span className="ratecount norate">Pas encore noté</span>}
        </div>
        <p className="cdesc">{p.description}</p>
        <div className="cmeta">
          <span className="pill">v<b>{p.version}</b></span>
          <span className="pill ctx">{p.contextPath}</span>
          {p.authType === 'oauth2' && <span className="pill oauth">OAuth2</span>}
        </div>
        <div className="cfoot">
          <div className="ctags">{p.tags.slice(0, 2).map(t => <span key={t} className="ctag">{t}</span>)}</div>
          <button className="subbtn" onClick={() => onSubscribe(p)}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <path d="M12 5v14M5 12h14" strokeLinecap="round"/>
            </svg>
            S'abonner
          </button>
        </div>
      </div>
    </article>
  )
}
