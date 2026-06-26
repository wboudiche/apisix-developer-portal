import { Routes, Route, Navigate } from 'react-router-dom'
import { CatalogPage } from './pages/CatalogPage'
import { LoginPage } from './pages/LoginPage'
import { RegisterPage } from './pages/RegisterPage'
import { ApplicationsIndex } from './pages/application/ApplicationsIndex'
import { AppDetailPage } from './pages/application/AppDetailPage'
import { AdminGuard } from './admin/AdminGuard'
import { ProductsPage } from './pages/admin/ProductsPage'
import { PlansPage } from './pages/admin/PlansPage'
import { ApprovalsPage } from './pages/admin/ApprovalsPage'
import { ProductDetailPage } from './pages/ProductDetailPage'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<CatalogPage />} />
      <Route path="/apis/:slug" element={<ProductDetailPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/applications" element={<ApplicationsIndex />} />
      <Route path="/applications/:id" element={<AppDetailPage />} />
      <Route path="/admin" element={<Navigate to="/admin/products" replace />} />
      <Route path="/admin/products" element={<AdminGuard><ProductsPage /></AdminGuard>} />
      <Route path="/admin/plans" element={<AdminGuard><PlansPage /></AdminGuard>} />
      <Route path="/admin/approvals" element={<AdminGuard><ApprovalsPage /></AdminGuard>} />
    </Routes>
  )
}
