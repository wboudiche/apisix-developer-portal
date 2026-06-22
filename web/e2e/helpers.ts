import { expect, type Locator, type Page } from '@playwright/test'
import { PAGE_SIZE } from './seed-data'

// Locators for the shared <Pagination> prev/next component, which renders
// "Préc."  /  "Page X · N au total"  /  "Suiv.".
export function pager(page: Page) {
  return {
    prev: page.getByRole('button', { name: 'Préc.' }),
    next: page.getByRole('button', { name: 'Suiv.' }),
    label: page.getByText(/Page \d+ · \d+ au total/),
  }
}

// Reads the "N au total" count the pager advertises.
export async function pagerTotal(page: Page): Promise<number> {
  const text = await pager(page).label.textContent()
  const m = text?.match(/·\s*(\d+)\s*au total/)
  if (!m) throw new Error(`pager total not found in "${text}"`)
  return Number(m[1])
}

// Drives the full prev/next lifecycle of a paginated list and asserts the
// boundaries: page 1 is full with Préc. disabled, each Suiv. refetches with a
// page=<k> query param and advances the label, the last page is partial with
// Suiv. disabled, and Préc. returns to page 1.
//
// `items` counts the rendered rows/cards; `apiPathPart` distinguishes which list
// request to watch (e.g. '/api/products', '/api/admin/plans').
export async function expectPaginationLifecycle(
  page: Page, items: Locator, apiPathPart: string,
): Promise<number> {
  const p = pager(page)
  await expect(p.label).toBeVisible()

  const total = await pagerTotal(page)
  expect(total).toBeGreaterThan(PAGE_SIZE) // seed guarantees a multi-page list
  const lastPage = Math.ceil(total / PAGE_SIZE)

  // Page 1: full page, Préc. disabled, Suiv. enabled.
  await expect(p.label).toHaveText(/Page 1 ·/)
  await expect(p.prev).toBeDisabled()
  await expect(p.next).toBeEnabled()
  await expect(items).toHaveCount(PAGE_SIZE)

  // Walk forward; each Suiv. must trigger a refetch carrying page=<target>.
  for (let target = 2; target <= lastPage; target++) {
    await Promise.all([
      page.waitForRequest(r => {
        if (!r.url().includes(apiPathPart)) return false
        return new URL(r.url()).searchParams.get('page') === String(target)
      }),
      p.next.click(),
    ])
    await expect(p.label).toHaveText(new RegExp(`Page ${target} ·`))
  }

  // Last page: Suiv. disabled, partial item count.
  await expect(p.next).toBeDisabled()
  await expect(items).toHaveCount(total - PAGE_SIZE * (lastPage - 1))

  // Back to the start.
  await p.prev.click()
  await expect(p.label).toHaveText(/Page 1 ·/)
  await expect(p.prev).toBeDisabled()

  return total
}
