import { test } from '@playwright/test'
import { ADMIN_STATE } from './seed-data'
import { expectPaginationLifecycle, goto } from './helpers'

test.use({ storageState: ADMIN_STATE })

test.describe('Admin approvals pagination', () => {
  test('pending subscription queue pages through and refetches with page param', async ({ page }) => {
    await goto(page, '/admin/approvals')
    const cards = page.locator('.sub-card')
    await expectPaginationLifecycle(page, cards, '/api/admin/subscriptions')
  })
})
