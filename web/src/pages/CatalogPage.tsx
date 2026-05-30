import { useEffect, useMemo, useState } from 'react'
import { getProducts } from '../api/client'
import type { Product } from '../api/types'
import { TopBar } from '../components/TopBar'
import { CategoryRail } from '../components/CategoryRail'
import { ApiCard } from '../components/ApiCard'
import '../styles/catalog.css'

export function CatalogPage() {
  const [products, setProducts] = useState<Product[]>([])
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let alive = true
    setLoading(true)
    getProducts({ search: search || undefined, category: category || undefined })
      .then(p => { if (alive) setProducts(p) })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [search, category])

  const categories = useMemo(() => {
    const counts: Record<string, number> = {}
    products.forEach(p => { counts[p.category] = (counts[p.category] ?? 0) + 1 })
    return Object.entries(counts).map(([name, count]) => ({ name, count }))
  }, [products])

  return (
    <>
      <TopBar search={search} onSearch={setSearch} />
      <div className="layout">
        <CategoryRail categories={categories} active={category} onPick={setCategory} />
        <main className="content">
          <div className="chead"><div className="titlewrap"><h1>Catalogue d'API</h1>
            <p className="rescount"><b>{products.length}</b> API{products.length > 1 ? 's' : ''}</p></div></div>
          <div className="grid">
            {products.map(p => <ApiCard key={p.id} p={p} />)}
          </div>
          {!loading && products.length === 0 && <p className="rescount">Aucune API ne correspond.</p>}
        </main>
      </div>
    </>
  )
}
