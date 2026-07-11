import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthProvider, useAuth } from './AuthProvider'
import * as client from '../api/client'

beforeEach(() => { localStorage.clear(); vi.restoreAllMocks() })

function Probe() {
  const { user, login, logout } = useAuth()
  return (
    <div>
      <span data-testid="who">{user ? user.email : 'anon'}</span>
      <button onClick={() => login('a@b.c', 'pw12345678')}>login</button>
      <button onClick={logout}>logout</button>
    </div>
  )
}

describe('AuthProvider', () => {
  it('logs in, exposes user, persists token, then logs out', async () => {
    vi.spyOn(client, 'login').mockResolvedValue({
      user: { id: 1, email: 'a@b.c', name: '', role: 'developer' }, token: 'jwt-123',
    })
    render(<AuthProvider><Probe /></AuthProvider>)
    expect(screen.getByTestId('who').textContent).toBe('anon')

    await userEvent.click(screen.getByText('login'))
    await waitFor(() => expect(screen.getByTestId('who').textContent).toBe('a@b.c'))
    expect(localStorage.getItem('token')).toBe('jwt-123')

    await userEvent.click(screen.getByText('logout'))
    await waitFor(() => expect(screen.getByTestId('who').textContent).toBe('anon'))
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('register returns true and stores no token when verification is required', async () => {
    vi.spyOn(client, 'register').mockResolvedValue({
      user: { id: 1, email: 'd@x.io', name: 'D', role: 'developer', language: 'fr' },
      verificationRequired: true,
    })
    let result: boolean | undefined
    function RegisterProbe() {
      const { register } = useAuth()
      return <button onClick={async () => { result = await register('d@x.io', 'longenough', 'D') }}>register</button>
    }
    render(<AuthProvider><RegisterProbe /></AuthProvider>)
    await userEvent.click(screen.getByText('register'))
    await waitFor(() => expect(result).toBe(true))
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('register returns false and logs in when no verification is required', async () => {
    vi.spyOn(client, 'register').mockResolvedValue({
      user: { id: 1, email: 'd@x.io', name: 'D', role: 'developer', language: 'fr' },
      token: 'jwt-token',
    })
    let result: boolean | undefined
    function RegisterProbe() {
      const { register } = useAuth()
      return <button onClick={async () => { result = await register('d@x.io', 'longenough', 'D') }}>register</button>
    }
    render(<AuthProvider><RegisterProbe /></AuthProvider>)
    await userEvent.click(screen.getByText('register'))
    await waitFor(() => expect(result).toBe(false))
    expect(localStorage.getItem('token')).toBe('jwt-token')
  })
})
