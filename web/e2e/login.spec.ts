import { expect, test } from '@playwright/test'
import { ADMIN } from './seed-data'
import { goto } from './helpers'

// Smoke: the admin signs in through the real form and reaches the admin area
// (which the AdminGuard would bounce a non-admin out of). This also proves the
// seeded admin promotion took effect end to end.
test('admin logs in through the UI and reaches the admin area', async ({ page }) => {
  await goto(page, '/login')
  await page.getByLabel('Email', { exact: true }).fill(ADMIN.email)
  await page.getByLabel('Mot de passe', { exact: true }).fill(ADMIN.password)
  await page.getByRole('button', { name: 'Se connecter' }).click()

  await expect(page).toHaveURL('/') // nav('/') after a successful login

  await goto(page, '/admin/products')
  await expect(page).toHaveURL(/\/admin\/products$/) // not redirected by AdminGuard
  await expect(page.getByRole('heading', { level: 1, name: 'Produits' })).toBeVisible()
})
