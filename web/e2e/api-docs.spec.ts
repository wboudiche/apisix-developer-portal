import { expect, test } from '@playwright/test'
import { ADMIN_STATE } from './seed-data'
import { goto } from './helpers'

// The e2e stack runs Postgres + Go API + Vite but has NO APISIX gateway.
// This spec covers the gateway-independent behaviour:
//  - a seeded product whose openapiSpec is non-empty renders Scalar docs
//  - a seeded product with no spec shows the "bientôt disponible" placeholder
//
// global-setup seeds e2e-product-00 with E2E_SPEC and all others with ''.

test.use({ storageState: ADMIN_STATE })

test.describe('API docs page', () => {
  test('renders Scalar docs for a product with a spec', async ({ page }) => {
    await goto(page, '/catalog/e2e-product-00')

    // Product header must be visible first.
    await expect(
      page.getByRole('heading', { level: 1, name: /E2E Product 00/ }),
    ).toBeVisible()

    // Scalar renders info.title into the page; wait generously because the
    // lazy-loaded bundle can be slow on first load.
    await expect(page.getByText('E2E Spec API')).toBeVisible({ timeout: 15000 })

    // The no-spec placeholder must NOT appear.
    await expect(page.getByText('Documentation bientôt disponible')).toHaveCount(0)
  })

  test('shows placeholder for a product without a spec', async ({ page }) => {
    await goto(page, '/catalog/e2e-product-01')

    await expect(
      page.getByRole('heading', { level: 1, name: /E2E Product 01/ }),
    ).toBeVisible()

    await expect(page.getByText('Documentation bientôt disponible')).toBeVisible()
  })
})
