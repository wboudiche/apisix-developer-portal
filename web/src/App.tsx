import { Routes, Route, Navigate } from 'react-router-dom'
import { CatalogPage } from './pages/CatalogPage'
import { LoginPage } from './pages/LoginPage'
import { RegisterPage } from './pages/RegisterPage'
import { VerifyEmailPage } from './pages/VerifyEmailPage'
import { ApplicationsIndex } from './pages/application/ApplicationsIndex'
import { AppDetailPage } from './pages/application/AppDetailPage'
import { AdminGuard } from './admin/AdminGuard'
import { ProductsPage } from './pages/admin/ProductsPage'
import { PlansPage } from './pages/admin/PlansPage'
import { ApprovalsPage } from './pages/admin/ApprovalsPage'
import { InvoicesPage } from './pages/admin/InvoicesPage'
import { SettingsPage } from './pages/admin/SettingsPage'
import { ProductDetailPage } from './pages/ProductDetailPage'
import TeamsPage from './pages/teams/TeamsPage'
import BillingPage from './pages/billing/BillingPage'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<CatalogPage />} />
      <Route path="/catalog/:slug" element={<ProductDetailPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/verify-email" element={<VerifyEmailPage />} />
      <Route path="/applications" element={<ApplicationsIndex />} />
      <Route path="/applications/:id" element={<AppDetailPage />} />
      <Route path="/teams" element={<TeamsPage />} />
      <Route path="/billing" element={<BillingPage />} />
      <Route path="/admin" element={<Navigate to="/admin/products" replace />} />
      <Route path="/admin/products" element={<AdminGuard><ProductsPage /></AdminGuard>} />
      <Route path="/admin/plans" element={<AdminGuard><PlansPage /></AdminGuard>} />
      <Route path="/admin/approvals" element={<AdminGuard><ApprovalsPage /></AdminGuard>} />
      <Route path="/admin/invoices" element={<AdminGuard><InvoicesPage /></AdminGuard>} />
      <Route path="/admin/settings" element={<AdminGuard><SettingsPage /></AdminGuard>} />
    </Routes>
  )
}
