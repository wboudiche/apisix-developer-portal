import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ImportModal } from './ImportModal'
import { AuthProvider } from '../../auth/AuthProvider'
import * as api from '../../api/client'
import type { AdminProduct } from '../../api/types'

const draft: AdminProduct = {
  name: 'Imported API', slug: 'imported', category: 'Finance', version: '1.0.0',
  contextPath: '/v1', description: '', tags: ['Finance'], icon: '', upstreamUrl: 'api.example.com:443', published: false,
}

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
})
afterEach(() => vi.restoreAllMocks())

function renderModal(onImported = vi.fn(), onClose = vi.fn()) {
  render(<AuthProvider><ImportModal open onClose={onClose} onImported={onImported} /></AuthProvider>)
  return { onImported, onClose }
}

it('imports from a URL and calls onImported with the draft', async () => {
  const spy = vi.spyOn(api, 'adminImportProduct').mockResolvedValue(draft)
  const { onImported } = renderModal()
  await userEvent.click(screen.getByRole('tab', { name: /URL/i }))
  await userEvent.type(screen.getByPlaceholderText(/https/i), 'https://api.example.com/openapi.json')
  await userEvent.click(screen.getByRole('button', { name: /Importer/i }))
  await waitFor(() => expect(onImported).toHaveBeenCalledWith(draft))
  expect(spy).toHaveBeenCalledWith('jwt', { url: 'https://api.example.com/openapi.json' })
})

it('shows the backend error message on failure', async () => {
  vi.spyOn(api, 'adminImportProduct').mockRejectedValue(new api.ApiError('spec could not be parsed', 422))
  renderModal()
  await userEvent.click(screen.getByRole('tab', { name: /URL/i }))
  await userEvent.type(screen.getByPlaceholderText(/https/i), 'https://x/y')
  await userEvent.click(screen.getByRole('button', { name: /Importer/i }))
  expect(await screen.findByRole('alert')).toHaveTextContent(/spec could not be parsed/i)
})
