import type { ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'

// AdminGuard renders its children only for an authenticated admin; everyone else
// is redirected to the catalog.
export function AdminGuard({ children }: { children: ReactNode }) {
  const { user } = useAuth()
  if (!user || user.role !== 'admin') return <Navigate to="/" replace />
  return <>{children}</>
}
