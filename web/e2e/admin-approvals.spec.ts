import { test } from '@playwright/test'
import { ADMIN_STATE } from './seed-data'
import { expectPaginationLifecycle } from './helpers'

test.use({ storageState: ADMIN_STATE })

test.describe('Admin approvals pagination', () => {
  test('pending subscription queue pages through and refetches with page param', async ({ page }) => {
    await page.goto('/admin/approvals')
    const cards = page.locator('.sub-card')
    await expectPaginationLifecycle(page, cards, '/api/admin/subscriptions')
  })
})
