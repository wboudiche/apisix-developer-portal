import { test } from '@playwright/test'
import { ADMIN_STATE } from './seed-data'
import { expectPaginationLifecycle, goto } from './helpers'

test.use({ storageState: ADMIN_STATE })

test.describe('Admin products pagination', () => {
  test('product list pages through and refetches with page param', async ({ page }) => {
    await goto(page, '/admin/products')
    const rows = page.locator('.rows .row')
    await expectPaginationLifecycle(page, rows, '/api/admin/products')
  })
})
