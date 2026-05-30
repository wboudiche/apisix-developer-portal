import { Link } from 'react-router-dom'
import { TopBar } from './TopBar'

// AdminNav renders the top bar plus the admin section sub-navigation. `active`
// is one of 'products' | 'plans' | 'approvals'.
export function AdminNav({ active }: { active: 'products' | 'plans' | 'approvals' }) {
  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <nav className="nav-tabs" style={{ padding: '12px 28px', gap: 14 }}>
        <Link className={active === 'products' ? 'active' : ''} to="/admin/products">Produits</Link>
        <Link className={active === 'plans' ? 'active' : ''} to="/admin/plans">Plans</Link>
        <Link className={active === 'approvals' ? 'active' : ''} to="/admin/approvals">Abonnements</Link>
      </nav>
    </>
  )
}
