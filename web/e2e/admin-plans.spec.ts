import { test } from '@playwright/test'
import { ADMIN_STATE } from './seed-data'
import { expectPaginationLifecycle, goto } from './helpers'

test.use({ storageState: ADMIN_STATE })

test.describe('Admin plans pagination', () => {
  test('plan list pages through and refetches with page param', async ({ page }) => {
    await goto(page, '/admin/plans')
    const rows = page.locator('.rows .row')
    await expectPaginationLifecycle(page, rows, '/api/admin/plans')
  })
})
