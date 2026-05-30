import { Routes, Route } from 'react-router-dom'
import { CatalogPage } from './pages/CatalogPage'
import { LoginPage } from './pages/LoginPage'
import { RegisterPage } from './pages/RegisterPage'
import { ApplicationsPage } from './pages/ApplicationsPage'
import { AdminGuard } from './admin/AdminGuard'
import { AdminProductsPage } from './pages/AdminProductsPage'
import { AdminPlansPage } from './pages/AdminPlansPage'
import { AdminApprovalsPage } from './pages/AdminApprovalsPage'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<CatalogPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/applications" element={<ApplicationsPage />} />
      <Route path="/admin/products" element={<AdminGuard><AdminProductsPage /></AdminGuard>} />
      <Route path="/admin/plans" element={<AdminGuard><AdminPlansPage /></AdminGuard>} />
      <Route path="/admin/approvals" element={<AdminGuard><AdminApprovalsPage /></AdminGuard>} />
    </Routes>
  )
}
