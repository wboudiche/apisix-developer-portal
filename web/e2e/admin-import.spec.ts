import { Buffer } from 'node:buffer'
import * as http from 'node:http'
import type { AddressInfo } from 'node:net'
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

  // Each bad input must come back 422 and surface the backend message inline,
  // with no Composer opened and nothing persisted. Covers: malformed JSON syntax
  // (unbalanced braces), broken YAML syntax (bad indentation), well-formed-but-
  // not-a-spec, and a valid document missing the required info.title.
  const BAD_INPUTS: { label: string; name: string; mimeType: string; body: string }[] = [
    { label: 'malformed JSON syntax', name: 'broken.json', mimeType: 'application/json', body: '{"openapi": "3.0.0", "info": {' },
    { label: 'malformed YAML syntax', name: 'broken.yaml', mimeType: 'application/yaml', body: 'openapi: 3.0.0\ninfo: [unbalanced' },
    { label: 'well-formed but not a spec', name: 'not-a-spec.json', mimeType: 'application/json', body: '{"hello":"world"}' },
    { label: 'valid spec missing info.title', name: 'no-title.json', mimeType: 'application/json', body: '{"openapi":"3.0.0","info":{"version":"1.0.0"}}' },
  ]

  for (const bad of BAD_INPUTS) {
    test(`rejects ${bad.label} with an inline error and opens no composer`, async ({ page }) => {
      await goto(page, '/admin/products')

      await page.getByRole('button', { name: 'Importer une API' }).click()
      const dialog = page.getByRole('dialog', { name: 'Importer une API' })
      await expect(dialog).toBeVisible()

      await dialog.getByLabel('Fichier de spécification').setInputFiles({
        name: bad.name, mimeType: bad.mimeType, buffer: Buffer.from(bad.body),
      })
      const [res] = await Promise.all([
        page.waitForResponse(r =>
          r.url().includes('/api/admin/products/import') && r.request().method() === 'POST'),
        dialog.getByRole('button', { name: 'Importer', exact: true }).click(),
      ])
      expect(res.status()).toBe(422)

      // The modal stays open and surfaces the backend's parse message inline; the
      // Composer is never opened (nothing persisted).
      await expect(dialog.getByRole('alert')).toHaveText(/parsed|OpenAPI|Swagger/i)
      await expect(page.getByText('Créer un produit')).toHaveCount(0)
    })
  }

  // The URL tab makes the Go backend fetch the spec server-side. The e2e stack
  // runs with UPSTREAM_ALLOW_PRIVATE=1, so a loopback test server is reachable
  // and passes the SSRF guard (the guard's private-range rejection is covered by
  // the Go unit tests, which can't set allowPrivate).
  test.describe('from a URL', () => {
    let server: http.Server
    let baseUrl: string

    test.beforeAll(async () => {
      server = http.createServer((_req, res) => {
        res.writeHead(200, { 'Content-Type': 'application/yaml' })
        res.end(VALID_SPEC)
      })
      await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve))
      baseUrl = `http://127.0.0.1:${(server.address() as AddressInfo).port}`
    })

    test.afterAll(async () => {
      await new Promise<void>(resolve => server.close(() => resolve()))
    })

    test('fetches a spec from a URL and pre-fills the composer', async ({ page }) => {
      await goto(page, '/admin/products')

      await page.getByRole('button', { name: 'Importer une API' }).click()
      const dialog = page.getByRole('dialog', { name: 'Importer une API' })
      await expect(dialog).toBeVisible()

      await dialog.getByRole('tab', { name: 'URL' }).click()
      await dialog.getByLabel('URL de la spécification').fill(`${baseUrl}/openapi.yaml`)
      const [res] = await Promise.all([
        page.waitForResponse(r =>
          r.url().includes('/api/admin/products/import') && r.request().method() === 'POST'),
        dialog.getByRole('button', { name: 'Importer', exact: true }).click(),
      ])
      expect(res.status()).toBe(200)

      await expect(page.getByText('Créer un produit')).toBeVisible()
      await expect(page.getByLabel('Nom')).toHaveValue('Petstore Import API')
      await expect(page.getByLabel('Version')).toHaveValue('2.0.0')
      await expect(page.getByLabel('Context path')).toHaveValue('/api/v3')
      await expect(page.getByLabel(/Upstream/)).toHaveValue('petstore.example.com:443')
    })

    test('rejects an unreachable URL with an inline error and opens no composer', async ({ page }) => {
      await goto(page, '/admin/products')

      await page.getByRole('button', { name: 'Importer une API' }).click()
      const dialog = page.getByRole('dialog', { name: 'Importer une API' })
      await expect(dialog).toBeVisible()

      await dialog.getByRole('tab', { name: 'URL' }).click()
      // Port 1 on loopback refuses the connection, so the server-side fetch fails.
      await dialog.getByLabel('URL de la spécification').fill('http://127.0.0.1:1/openapi.yaml')
      const [res] = await Promise.all([
        page.waitForResponse(r =>
          r.url().includes('/api/admin/products/import') && r.request().method() === 'POST'),
        dialog.getByRole('button', { name: 'Importer', exact: true }).click(),
      ])
      expect(res.status()).toBe(422)

      await expect(dialog.getByRole('alert')).toBeVisible()
      await expect(page.getByText('Créer un produit')).toHaveCount(0)
    })
  })
})
