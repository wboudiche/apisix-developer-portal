import { Suspense, lazy, useEffect, useState } from 'react'
import { Link, useParams, useNavigate } from 'react-router-dom'
import { getProduct, getProductSpec, getTryContext, getChangelog } from '../api/client'
import type { ChangelogEntry, Product, TryApp } from '../api/types'
import { useAuth } from '../auth/AuthProvider'
import { TopBar } from '../components/TopBar'
import { SubscribeModal } from '../components/SubscribeModal'
import { ApiIcon, categoryDotColor } from '../components/apiIcons'
import { LifecycleBadge } from '../components/LifecycleBadge'
import { Reviews } from '../components/Reviews'
import '../styles/productdetail.css'

const ScalarDocs = lazy(() => import('../components/ScalarDocs').then(m => ({ default: m.ScalarDocs })))

export function ProductDetailPage() {
  const { slug = '' } = useParams()
  const { user, token } = useAuth()
  const nav = useNavigate()
  const [product, setProduct] = useState<Product | null>(null)
  const [spec, setSpec] = useState<string | null>(null)
  const [changelog, setChangelog] = useState<ChangelogEntry[]>([])
  const [loaded, setLoaded] = useState(false)
  const [err, setErr] = useState('')
  const [subOpen, setSubOpen] = useState(false)
  const [apps, setApps] = useState<TryApp[]>([])
  const [appId, setAppId] = useState<number | null>(null)
  const [tryLoaded, setTryLoaded] = useState(false)
  const [sandboxAvailable, setSandboxAvailable] = useState(false)
  const [tryMode, setTryMode] = useState<'prod' | 'sandbox'>('prod')

  useEffect(() => {
    let alive = true
    setLoaded(false)
    Promise.all([getProduct(slug), getProductSpec(slug).catch(() => null)])
      .then(([p, s]) => { if (alive) { setProduct(p); setSpec(s) } })
      .catch(() => { if (alive) setErr('Produit introuvable.') })
      .finally(() => { if (alive) setLoaded(true) })
    getChangelog(slug).then(c => { if (alive) setChangelog(c) }).catch(() => {})
    return () => { alive = false }
  }, [slug])

  useEffect(() => {
    if (!token) return
    let alive = true
    setTryLoaded(false)
    getTryContext(token, slug)
      .then(r => { if (alive) { setApps(r.apps); setAppId(r.apps[0]?.id ?? null); setSandboxAvailable(r.sandboxAvailable ?? false) } })
      .catch(() => { if (alive) { setApps([]); setAppId(null); setSandboxAvailable(false) } })
      .finally(() => { if (alive) setTryLoaded(true) })
    return () => { alive = false }
  }, [token, slug])

  const serverUrl = appId != null
    ? `/api/try/${slug}/${appId}${tryMode === 'sandbox' && sandboxAvailable ? '/sandbox' : ''}`
    : undefined

  const blocked = product?.lifecycleStatus === 'deprecated' || product?.lifecycleStatus === 'sunset'

  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <div className="apidetail">
        <div className="crumbs"><Link to="/">Catalogue</Link> <span>/</span> <b>{product?.name ?? slug}</b></div>
        {err && <p className="autherr" role="alert">{err}</p>}
        {product && (
          <>
            <header className="apihead">
              <span className="glyph" style={{ background: categoryDotColor(product.category) }}><ApiIcon name={product.icon} /></span>
              <div className="htext">
                <h1>{product.name}</h1>
                <p className="sub"><span className="cat">{product.category}</span> · v{product.version} · {product.ratingCount > 0 ? <>★ {product.rating.toFixed(1)} ({product.ratingCount})</> : 'Pas encore noté'}{product.authType === 'oauth2' && <> · <span className="pill oauth">OAuth2</span></>}{product.lifecycleStatus && product.lifecycleStatus !== 'active' && <> · <LifecycleBadge status={product.lifecycleStatus} /></>}</p>
                {product.description && <p className="desc">{product.description}</p>}
                <div className="tags">{product.tags.map(t => <span key={t} className="tag">{t}</span>)}</div>
              </div>
              <button className="btn btn-primary" disabled={blocked} title={blocked ? "Cette API n'accepte plus de nouveaux abonnements" : undefined}
                onClick={() => { if (blocked) return; user ? setSubOpen(true) : nav('/login') }}>S'abonner</button>
            </header>
            {product.lifecycleStatus === 'deprecated' && <div className="notice deprecated">Cette API est dépréciée et n'accepte plus de nouveaux abonnements.</div>}
            {product.lifecycleStatus === 'sunset' && <div className="notice sunset">Cette API sera retirée{product.sunsetDate ? ` le ${product.sunsetDate}` : ''}.</div>}

            {token && apps.length > 1 && (
              <div className="try-picker">
                <label htmlFor="app-picker">Essayer avec :</label>
                <select id="app-picker" value={appId ?? ''} onChange={e => setAppId(Number(e.target.value))}>
                  {apps.map(a => <option key={a.id} value={a.id}>{a.name}</option>)}
                </select>
              </div>
            )}
            {token && appId != null && sandboxAvailable && (
              <div className="try-mode" role="group" aria-label="Environnement">
                <button type="button" className={tryMode === 'prod' ? 'on' : ''} onClick={() => setTryMode('prod')} aria-pressed={tryMode === 'prod'}>Production</button>
                <button type="button" className={tryMode === 'sandbox' ? 'on' : ''} onClick={() => setTryMode('sandbox')} aria-pressed={tryMode === 'sandbox'}>Sandbox</button>
              </div>
            )}
            {token && tryLoaded && apps.length === 0 && (
              <div className="try-banner">Abonnez-vous pour essayer les requêtes via la passerelle.</div>
            )}

            {loaded && spec && (
              <Suspense fallback={<p className="docs-loading">Chargement de la documentation…</p>}>
                <ScalarDocs spec={spec} serverUrl={serverUrl} />
              </Suspense>
            )}
            {loaded && !spec && (
              <div className="docs-empty"><h3>Documentation bientôt disponible</h3>
                <p>Aucune spécification OpenAPI n'est encore attachée à cette API.</p></div>
            )}
            {changelog.length > 0 && (
              <section className="changelog">
                <h2>Journal des modifications</h2>
                <ul>{changelog.map(e => (
                  <li key={e.id}>
                    <span className={`ctag ${e.kind}`}>{e.kind}</span>
                    <b>{e.version}</b> <span className="cdate mono">{e.date}</span>
                    {e.notes && <p>{e.notes}</p>}
                  </li>
                ))}</ul>
              </section>
            )}
            <Reviews slug={slug} token={token} />
          </>
        )}
      </div>
      {subOpen && product && <SubscribeModal product={product} onClose={() => setSubOpen(false)} />}
    </>
  )
}
