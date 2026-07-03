import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { TopBar } from '../../components/TopBar'
import { useAuth } from '../../auth/AuthProvider'
import { adminGetProducts, adminGetPlans, adminGetSubscriptions } from '../../api/client'
import { useT } from '../../i18n/LanguageProvider'
import '../../styles/admin.css'

export type AdminTab = 'products' | 'plans' | 'approvals' | 'invoices'

// Pill sub-nav with real count badges + blueprint page head. A page passes the
// count it already knows (its own list length) via `counts`; the shell fetches
// the others once on mount.
export function AdminShell({ active, title, description, action, counts, children }: {
  active: AdminTab
  title: string
  description: ReactNode
  action?: ReactNode
  counts?: { products?: number; plans?: number; pending?: number }
  children: ReactNode
}) {
  const { token } = useAuth()
  const t = useT()
  const [fetched, setFetched] = useState<{ products?: number; plans?: number; pending?: number }>({})
  // Which keys the page provides is fixed per call site — capture at mount.
  const provided = useRef({
    products: counts?.products !== undefined,
    plans: counts?.plans !== undefined,
    pending: counts?.pending !== undefined,
  })

  useEffect(() => {
    if (!token) return
    let alive = true
    if (!provided.current.products)
      adminGetProducts(token).then(r => { if (alive) setFetched(f => ({ ...f, products: r.total })) }).catch(() => {})
    if (!provided.current.plans)
      adminGetPlans(token).then(r => { if (alive) setFetched(f => ({ ...f, plans: r.total })) }).catch(() => {})
    if (!provided.current.pending)
      adminGetSubscriptions(token, 'pending').then(r => { if (alive) setFetched(f => ({ ...f, pending: r.total })) }).catch(() => {})
    return () => { alive = false }
  }, [token])

  const n = {
    products: counts?.products ?? fetched.products,
    plans: counts?.plans ?? fetched.plans,
    pending: counts?.pending ?? fetched.pending,
  }
  const badge = (v?: number) => v === undefined ? null : <span className="ct">{v}</span>

  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <div className="adminpage">
        <nav className="subnav" aria-label={t('admin.navAriaLabel')}>
          <Link className={active === 'products' ? 'active' : ''} to="/admin/products">{t('admin.productsLabel')} {badge(n.products)}</Link>
          <Link className={active === 'plans' ? 'active' : ''} to="/admin/plans">{t('admin.plansNavLabel')} {badge(n.plans)}</Link>
          <Link className={active === 'approvals' ? 'active' : ''} to="/admin/approvals">{t('admin.approvalsNavLabel')} {badge(n.pending)}</Link>
          <Link className={active === 'invoices' ? 'active' : ''} to="/admin/invoices">{t('admin.invoicesNavLabel')}</Link>
        </nav>
        <div className="apanel">
          <div className="phead">
            <div>
              <h1>{title}</h1>
              <p>{description}</p>
            </div>
            {action}
          </div>
          {children}
        </div>
      </div>
    </>
  )
}
