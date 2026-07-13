import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { adminPutSettings, adminGetSettings, adminResetSetting, adminTestSettings, SettingsSaveError, ApiError } from './client'

beforeEach(() => { localStorage.setItem('lang', 'fr') })
afterEach(() => { vi.restoreAllMocks() })

describe('adminGetSettings', () => {
  it('resolves with SettingsGroup[] on 200', async () => {
    const mockData = [
      {
        group: 'SMTP',
        items: [
          { key: 'SMTP_HOST', type: 'string', editable: true, secret: false, value: 'mail.example.com', set: true, source: 'db' as const, envDefault: null },
        ],
      },
    ]
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(mockData), { status: 200, headers: { 'Content-Type': 'application/json' } })
    )
    const result = await adminGetSettings('jwt')
    expect(result).toEqual(mockData)
  })
})

describe('adminPutSettings', () => {
  it('resolves on 204', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 204 }))
    await expect(adminPutSettings('jwt', { SMTP_HOST: 'x' })).resolves.toBeUndefined()
  })

  it('throws SettingsSaveError carrying field errors on 422', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(
      JSON.stringify({ fields: { SMTP_PORT: 'must be a port between 1 and 65535' } }), { status: 422 }))
    const err = await adminPutSettings('jwt', { SMTP_PORT: 'x' }).catch(e => e)
    expect(err).toBeInstanceOf(SettingsSaveError)
    expect(err.fields?.SMTP_PORT).toMatch(/port/)
  })

  it('throws SettingsSaveError carrying probe results on 422', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(
      JSON.stringify({ probe: [{ name: 'smtp', ok: false, detail: 'refused' }] }), { status: 422 }))
    const err = await adminPutSettings('jwt', { SMTP_HOST: 'bogus' }).catch(e => e)
    expect(err).toBeInstanceOf(SettingsSaveError)
    expect(err.probe?.[0].name).toBe('smtp')
  })

  it('routes non-422 errors through the generic parse path (ApiError, not SettingsSaveError)', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(
      JSON.stringify({ error: 'boom' }), { status: 500 }))
    const err = await adminPutSettings('jwt', { SMTP_HOST: 'x' }).catch(e => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).not.toBeInstanceOf(SettingsSaveError)
    expect(err.status).toBe(500)
  })
})

describe('adminResetSetting', () => {
  it('sends DELETE request on 204', async () => {
    const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 204 }))
    await adminResetSetting('jwt', 'SMTP_HOST')
    const [url, init] = f.mock.calls[0]
    expect(url).toBe('/api/admin/settings/SMTP_HOST')
    expect((init as RequestInit).method).toBe('DELETE')
  })

  it('URL-encodes a key containing reserved characters', async () => {
    const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 204 }))
    await adminResetSetting('jwt', 'FOO/BAR BAZ')
    expect(f.mock.calls[0][0]).toBe('/api/admin/settings/FOO%2FBAR%20BAZ')
  })
})

describe('adminTestSettings', () => {
  it('resolves with ProbeResult[] on 200', async () => {
    const mockProbes = [
      { name: 'smtp', ok: true, detail: 'OK' },
    ]
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(mockProbes), { status: 200, headers: { 'Content-Type': 'application/json' } })
    )
    const result = await adminTestSettings('jwt', { SMTP_HOST: 'mail.example.com' })
    expect(result).toEqual(mockProbes)
  })
})
