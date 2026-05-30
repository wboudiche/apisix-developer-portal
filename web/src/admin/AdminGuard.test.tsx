import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { AdminGuard } from './AdminGuard'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'

beforeEach(() => localStorage.clear())

function renderAt(role: string | null) {
  if (role) {
    localStorage.setItem('token', 'tok')
    localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: '', role }))
  }
  return render(
    <MemoryRouter initialEntries={['/admin']}>
      <ThemeProvider><AuthProvider>
        <Routes>
          <Route path="/" element={<div>CATALOG</div>} />
          <Route path="/admin" element={<AdminGuard><div>ADMIN AREA</div></AdminGuard>} />
        </Routes>
      </AuthProvider></ThemeProvider>
    </MemoryRouter>
  )
}

describe('AdminGuard', () => {
  it('renders children for an admin', () => {
    renderAt('admin')
    expect(screen.getByText('ADMIN AREA')).toBeInTheDocument()
  })
  it('redirects a developer to the catalog', () => {
    renderAt('developer')
    expect(screen.getByText('CATALOG')).toBeInTheDocument()
    expect(screen.queryByText('ADMIN AREA')).not.toBeInTheDocument()
  })
  it('redirects an anonymous visitor to the catalog', () => {
    renderAt(null)
    expect(screen.getByText('CATALOG')).toBeInTheDocument()
  })
})
