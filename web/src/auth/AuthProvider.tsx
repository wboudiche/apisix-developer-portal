import { createContext, useContext, useState, type ReactNode } from 'react'
import type { User } from '../api/types'
import * as api from '../api/client'

interface AuthState {
  user: User | null
  token: string | null
  login: (email: string, password: string) => Promise<void>
  register: (email: string, password: string, name: string) => Promise<void>
  logout: () => void
}

const AuthCtx = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('token'))
  const [user, setUser] = useState<User | null>(() => {
    const raw = localStorage.getItem('user')
    if (!raw) return null
    try { return JSON.parse(raw) as User } catch { return null }
  })

  function apply(res: { user: User; token: string }) {
    setUser(res.user); setToken(res.token)
    localStorage.setItem('token', res.token)
    localStorage.setItem('user', JSON.stringify(res.user))
  }

  const login = async (email: string, password: string) => apply(await api.login(email, password))
  const register = async (email: string, password: string, name: string) => apply(await api.register(email, password, name))
  const logout = () => {
    setUser(null); setToken(null)
    localStorage.removeItem('token'); localStorage.removeItem('user')
  }

  return <AuthCtx.Provider value={{ user, token, login, register, logout }}>{children}</AuthCtx.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthCtx)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
