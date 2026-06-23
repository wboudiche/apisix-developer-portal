import { expect, test } from '@playwright/test'
import { expectPaginationLifecycle, goto } from './helpers'

// Public catalogue grid — no auth required.
test.describe('Catalog pagination', () => {
  test('grid pages through products and shows the right total', async ({ page }) => {
    await goto(page, '/')

    const cards = page.locator('[data-testid="api-card"]')
    const total = await expectPaginationLifecycle(page, cards, '/api/products')

    // The header "N API(s)" count must agree with the pager's advertised total.
    await expect(page.locator('.titlewrap .rescount b')).toHaveText(String(total))
  })
})
