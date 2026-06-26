import { Suspense, lazy, useEffect, useState } from 'react'
import { Link, useParams, useNavigate } from 'react-router-dom'
import { getProduct, getProductSpec } from '../api/client'
import type { Product } from '../api/types'
import { useAuth } from '../auth/AuthProvider'
import { TopBar } from '../components/TopBar'
import { SubscribeModal } from '../components/SubscribeModal'
import { ApiIcon, categoryDotColor } from '../components/apiIcons'
import '../styles/productdetail.css'

const ScalarDocs = lazy(() => import('../components/ScalarDocs').then(m => ({ default: m.ScalarDocs })))

export function ProductDetailPage() {
  const { slug = '' } = useParams()
  const { user } = useAuth()
  const nav = useNavigate()
  const [product, setProduct] = useState<Product | null>(null)
  const [spec, setSpec] = useState<string | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [err, setErr] = useState('')
  const [subOpen, setSubOpen] = useState(false)

  useEffect(() => {
    let alive = true
    setLoaded(false)
    Promise.all([getProduct(slug), getProductSpec(slug).catch(() => null)])
      .then(([p, s]) => { if (alive) { setProduct(p); setSpec(s) } })
      .catch(() => { if (alive) setErr('Produit introuvable.') })
      .finally(() => { if (alive) setLoaded(true) })
    return () => { alive = false }
  }, [slug])

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
                <p className="sub"><span className="cat">{product.category}</span> · v{product.version} · ★ {product.rating}</p>
                {product.description && <p className="desc">{product.description}</p>}
                <div className="tags">{product.tags.map(t => <span key={t} className="tag">{t}</span>)}</div>
              </div>
              <button className="btn btn-primary" onClick={() => user ? setSubOpen(true) : nav('/login')}>S'abonner</button>
            </header>

            {loaded && spec && (
              <Suspense fallback={<p className="docs-loading">Chargement de la documentation…</p>}>
                <ScalarDocs spec={spec} />
              </Suspense>
            )}
            {loaded && !spec && (
              <div className="docs-empty"><h3>Documentation bientôt disponible</h3>
                <p>Aucune spécification OpenAPI n'est encore attachée à cette API.</p></div>
            )}
          </>
        )}
      </div>
      {subOpen && product && <SubscribeModal product={product} onClose={() => setSubOpen(false)} />}
    </>
  )
}
