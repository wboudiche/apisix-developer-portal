import { Buffer } from 'node:buffer'
import { expect, test } from '@playwright/test'
import { ADMIN_STATE } from './seed-data'
import { goto } from './helpers'

test.use({ storageState: ADMIN_STATE })

// A valid OpenAPI 3.x spec, intentionally YAML so the run exercises the real
// sigs.k8s.io/yaml decode path through the deployed Go binary (not just JSON).
// title -> Nom, version -> Version, servers[0].url path -> Context path and
// host:port -> Upstream, tags[0] -> Catégorie.
const VALID_SPEC = `openapi: 3.0.0
info:
  title: Petstore Import API
  version: 2.0.0
  description: Imported via the e2e suite
servers:
  - url: https://petstore.example.com/api/v3
tags:
  - name: Pets
`

// A Swagger 2.0 spec, intentionally JSON so the run also covers the JSON decode
// path and the 2.0 mapping (host + basePath + schemes -> Context path + Upstream)
// through the real binary. host -> Upstream host:port (https default 443),
// basePath -> Context path, tags[0] -> Catégorie.
const SWAGGER2_SPEC = JSON.stringify({
  swagger: '2.0',
  info: { title: 'Legacy Inventory API', version: '1.3.0' },
  host: 'inventory.example.com',
  basePath: '/v1',
  schemes: ['https'],
  tags: [{ name: 'Inventory' }],
})

test.describe('Admin OpenAPI import', () => {
  test('imports a spec file, pre-fills the composer, and creates the product', async ({ page }) => {
    await goto(page, '/admin/products')

    await page.getByRole('button', { name: 'Importer une API' }).click()
    const dialog = page.getByRole('dialog', { name: 'Importer une API' })
    await expect(dialog).toBeVisible()

    // File tab is the default. Upload the spec and watch the backend parse it.
    await dialog.getByLabel('Fichier de spécification').setInputFiles({
      name: 'petstore.yaml',
      mimeType: 'application/yaml',
      buffer: Buffer.from(VALID_SPEC),
    })
    const [importRes] = await Promise.all([
      page.waitForResponse(r =>
        r.url().includes('/api/admin/products/import') && r.request().method() === 'POST'),
      dialog.getByRole('button', { name: 'Importer', exact: true }).click(),
    ])
    expect(importRes.status()).toBe(200)

    // The existing Composer opens pre-filled as a CREATE (not an edit).
    await expect(page.getByText('Créer un produit')).toBeVisible()
    await expect(page.getByLabel('Nom')).toHaveValue('Petstore Import API')
    await expect(page.getByLabel('Version')).toHaveValue('2.0.0')
    await expect(page.getByLabel('Context path')).toHaveValue('/api/v3')
    await expect(page.getByLabel(/Upstream/)).toHaveValue('petstore.example.com:443')
    await expect(page.getByLabel('Catégorie')).toHaveValue('Pets')

    // Give the product a unique identity so the create succeeds even on a reused
    // DB / on retry (slugTouched is set after import, so editing Nom alone won't
    // touch the slug — set both explicitly).
    const uniq = Date.now()
    await page.getByLabel('Nom').fill(`Petstore E2E ${uniq}`)
    await page.getByLabel('Slug').fill(`petstore-e2e-${uniq}`)

    const [createRes] = await Promise.all([
      page.waitForResponse(r =>
        r.url().includes('/api/admin/products') &&
        !r.url().includes('/import') &&
        r.request().method() === 'POST'),
      page.getByRole('button', { name: 'Créer le produit' }).click(),
    ])
    expect(createRes.status()).toBe(201)
  })

  test('imports a Swagger 2.0 (JSON) spec and pre-fills the composer', async ({ page }) => {
    await goto(page, '/admin/products')

    await page.getByRole('button', { name: 'Importer une API' }).click()
    const dialog = page.getByRole('dialog', { name: 'Importer une API' })
    await expect(dialog).toBeVisible()

    await dialog.getByLabel('Fichier de spécification').setInputFiles({
      name: 'inventory.swagger.json',
      mimeType: 'application/json',
      buffer: Buffer.from(SWAGGER2_SPEC),
    })
    const [importRes] = await Promise.all([
      page.waitForResponse(r =>
        r.url().includes('/api/admin/products/import') && r.request().method() === 'POST'),
      dialog.getByRole('button', { name: 'Importer', exact: true }).click(),
    ])
    expect(importRes.status()).toBe(200)

    // host + basePath + schemes from the 2.0 spec map into the form.
    await expect(page.getByText('Créer un produit')).toBeVisible()
    await expect(page.getByLabel('Nom')).toHaveValue('Legacy Inventory API')
    await expect(page.getByLabel('Version')).toHaveValue('1.3.0')
    await expect(page.getByLabel('Context path')).toHaveValue('/v1')
    await expect(page.getByLabel(/Upstream/)).toHaveValue('inventory.example.com:443')
    await expect(page.getByLabel('Catégorie')).toHaveValue('Inventory')
  })

  test('rejects an unparseable spec with an inline error and opens no composer', async ({ page }) => {
    await goto(page, '/admin/products')

    await page.getByRole('button', { name: 'Importer une API' }).click()
    const dialog = page.getByRole('dialog', { name: 'Importer une API' })
    await expect(dialog).toBeVisible()

    await dialog.getByLabel('Fichier de spécification').setInputFiles({
      name: 'not-a-spec.json',
      mimeType: 'application/json',
      buffer: Buffer.from('this is not a spec at all'),
    })
    await dialog.getByRole('button', { name: 'Importer', exact: true }).click()

    // The modal stays open and surfaces the backend's 422 message inline; the
    // Composer is never opened.
    await expect(dialog.getByRole('alert')).toBeVisible()
    await expect(page.getByText('Créer un produit')).toHaveCount(0)
  })
})
