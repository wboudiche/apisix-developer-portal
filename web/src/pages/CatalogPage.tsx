import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { getProducts } from '../api/client'
import type { Product } from '../api/types'
import { TopBar } from '../components/TopBar'
import { CategoryRail } from '../components/CategoryRail'
import { ApiCard } from '../components/ApiCard'
import { SubscribeModal } from '../components/SubscribeModal'
import { useAuth } from '../auth/AuthProvider'
import '../styles/catalog.css'

export function CatalogPage() {
  const { user } = useAuth()
  const nav = useNavigate()
  const [products, setProducts] = useState<Product[]>([])
  const [allProducts, setAllProducts] = useState<Product[]>([])
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState<string | null>(null)
  const [tag, setTag] = useState<string | null>(null)
  const [sort, setSort] = useState<'rating' | 'alpha'>('rating')
  const [view, setView] = useState<'grid' | 'list'>('grid')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [modalProduct, setModalProduct] = useState<Product | null>(null)
  const [railOpen, setRailOpen] = useState(true)

  function handleSubscribe(p: Product) {
    if (!user) { nav('/login'); return }
    setModalProduct(p)
  }

  // Mount-only: fetch unfiltered catalog for stable category counts
  useEffect(() => {
    getProducts({}).then(setAllProducts).catch(() => { /* silent */ })
  }, [])

  useEffect(() => {
    let alive = true
    setLoading(true)
    setError('')
    getProducts({ search: search || undefined, category: category || undefined, tag: tag || undefined, sort })
      .then(p => { if (alive) setProducts(p) })
      .catch(() => { if (alive) { setProducts([]); setError('Impossible de charger le catalogue. Vérifiez que le service est démarré.') } })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [search, category, tag, sort])

  const categories = useMemo(() => {
    const counts: Record<string, number> = {}
    allProducts.forEach(p => { counts[p.category] = (counts[p.category] ?? 0) + 1 })
    return Object.entries(counts).map(([name, count]) => ({ name, count }))
  }, [allProducts])

  const tags = useMemo(() => Array.from(new Set(allProducts.flatMap(p => p.tags))).sort(), [allProducts])

  return (
    <>
      <TopBar search={search} onSearch={setSearch} onMenu={() => setRailOpen(o => !o)} />
      <div className="layout">
        <CategoryRail categories={categories} active={category} onPick={setCategory} tags={tags} activeTag={tag} onPickTag={setTag} open={railOpen} onClose={() => setRailOpen(false)} />
        <main className="content">
          <div className="chead">
            <div className="titlewrap">
              <h1>Catalogue d'API</h1>
              <p className="rescount"><b>{products.length}</b> API{products.length > 1 ? 's' : ''}</p>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginLeft: 'auto' }}>
              <select
                aria-label="Trier"
                value={sort}
                onChange={e => setSort(e.target.value as 'rating' | 'alpha')}
                style={{ fontSize: '13px', padding: '5px 10px', borderRadius: '8px', border: '1px solid var(--border-2)', background: 'var(--surface)', color: 'var(--fg)', cursor: 'pointer' }}
              >
                <option value="rating">Note</option>
                <option value="alpha">Alphabétique</option>
              </select>
              <button
                aria-label="Vue grille"
                onClick={() => setView('grid')}
                style={{ width: '32px', height: '32px', borderRadius: '8px', border: '1px solid var(--border-2)', background: view === 'grid' ? 'var(--accent-soft)' : 'var(--surface)', color: view === 'grid' ? 'var(--accent)' : 'var(--muted)', cursor: 'pointer', display: 'grid', placeItems: 'center' }}
              >
                <svg width="15" height="15" viewBox="0 0 15 15" fill="none"><rect x="1" y="1" width="5" height="5" rx="1" fill="currentColor"/><rect x="9" y="1" width="5" height="5" rx="1" fill="currentColor"/><rect x="1" y="9" width="5" height="5" rx="1" fill="currentColor"/><rect x="9" y="9" width="5" height="5" rx="1" fill="currentColor"/></svg>
              </button>
              <button
                aria-label="Vue liste"
                onClick={() => setView('list')}
                style={{ width: '32px', height: '32px', borderRadius: '8px', border: '1px solid var(--border-2)', background: view === 'list' ? 'var(--accent-soft)' : 'var(--surface)', color: view === 'list' ? 'var(--accent)' : 'var(--muted)', cursor: 'pointer', display: 'grid', placeItems: 'center' }}
              >
                <svg width="15" height="15" viewBox="0 0 15 15" fill="none"><rect x="1" y="2" width="13" height="2" rx="1" fill="currentColor"/><rect x="1" y="6.5" width="13" height="2" rx="1" fill="currentColor"/><rect x="1" y="11" width="13" height="2" rx="1" fill="currentColor"/></svg>
              </button>
            </div>
          </div>
          {error && <p className="autherr" role="alert">{error}</p>}
          <div className={`grid ${view === 'list' ? 'list' : ''}`}>
            {products.map(p => <ApiCard key={p.id} p={p} onSubscribe={handleSubscribe} />)}
          </div>
          {!loading && !error && products.length === 0 && <p className="rescount">Aucune API ne correspond.</p>}
        </main>
      </div>
      {modalProduct && <SubscribeModal product={modalProduct} onClose={() => setModalProduct(null)} />}
    </>
  )
}
