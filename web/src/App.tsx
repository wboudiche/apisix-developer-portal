import { Routes, Route } from 'react-router-dom'
import { CatalogPage } from './pages/CatalogPage'
import { LoginPage } from './pages/LoginPage'
import { RegisterPage } from './pages/RegisterPage'
import { ApplicationsPage } from './pages/ApplicationsPage'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<CatalogPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/applications" element={<ApplicationsPage />} />
    </Routes>
  )
}
