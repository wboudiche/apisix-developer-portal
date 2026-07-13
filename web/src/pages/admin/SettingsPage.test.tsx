import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SettingsPage } from './SettingsPage'
import { AuthProvider } from '../../auth/AuthProvider'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import * as api from '../../api/client'
import { SettingsSaveError } from '../../api/client'
import type { SettingsGroup } from '../../api/types'

const groups: SettingsGroup[] = [
  { group: 'server', items: [
    { key: 'JWT_SECRET', type: 'string', editable: false, secret: true, value: null, set: true, source: 'env', envDefault: null },
  ]},
  { group: 'smtp', items: [
    { key: 'SMTP_HOST', type: 'string', editable: true, secret: false, value: 'mailpit', set: true, source: 'db', envDefault: 'envhost' },
    { key: 'SMTP_PASSWORD', type: 'string', editable: true, secret: true, value: null, set: false, source: 'env', envDefault: null },
  ]},
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'adminGetSettings').mockResolvedValue(groups)
  vi.spyOn(api, 'adminGetProducts').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
  vi.spyOn(api, 'adminGetPlans').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
  vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
})

const renderPage = () => render(
  <MemoryRouter><LanguageProvider><AuthProvider><SettingsPage /></AuthProvider></LanguageProvider></MemoryRouter>
)

describe('SettingsPage', () => {
  it('renders groups, values, source badges, and masks secrets', async () => {
    renderPage()
    expect(await screen.findByDisplayValue('mailpit')).toBeInTheDocument()
    expect(screen.getByText('modifié')).toBeInTheDocument()
    expect(screen.queryByDisplayValue(/secret/i)).not.toBeInTheDocument()
    const jwt = screen.getByLabelText('JWT_SECRET') as HTMLInputElement
    expect(jwt.disabled).toBe(true)
  })

  it('saves the dirty draft and reloads', async () => {
    const put = vi.spyOn(api, 'adminPutSettings').mockResolvedValue(undefined)
    renderPage()
    const host = await screen.findByLabelText('SMTP_HOST')
    await userEvent.clear(host)
    await userEvent.type(host, 'newhost')
    await userEvent.click(screen.getByRole('button', { name: 'Enregistrer' }))
    await waitFor(() => expect(put).toHaveBeenCalledWith('jwt', { SMTP_HOST: 'newhost' }, false))
  })

  it('offers force-save when the probe fails', async () => {
    const put = vi.spyOn(api, 'adminPutSettings')
      .mockRejectedValueOnce(new SettingsSaveError('x', 422, undefined, [{ name: 'smtp', ok: false, detail: 'refused' }]))
      .mockResolvedValueOnce(undefined)
    renderPage()
    const host = await screen.findByLabelText('SMTP_HOST')
    await userEvent.clear(host)
    await userEvent.type(host, 'bogus')
    await userEvent.click(screen.getByRole('button', { name: 'Enregistrer' }))
    expect(await screen.findByText(/refused/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Enregistrer quand même' }))
    await waitFor(() => expect(put).toHaveBeenLastCalledWith('jwt', { SMTP_HOST: 'bogus' }, true))
  })

  it('resets an overridden key after confirmation', async () => {
    const reset = vi.spyOn(api, 'adminResetSetting').mockResolvedValue(undefined)
    renderPage()
    await screen.findByDisplayValue('mailpit')
    await userEvent.click(screen.getByRole('button', { name: /Rétablir/ }))
    // ConfirmModal path: it renders `role="dialog"` and its confirm button
    // defaults to `t('common.confirm')` ("Confirmer") — scope the query to
    // the dialog so it can't match the row's own "Rétablir" reset button.
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: /confirmer/i }))
    await waitFor(() => expect(reset).toHaveBeenCalledWith('jwt', 'SMTP_HOST'))
  })
})
