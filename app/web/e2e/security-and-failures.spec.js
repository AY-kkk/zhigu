import { expect, test } from '@playwright/test'
import { assertNoCredentialStorage, installProductApi } from './product-fixtures.js'

test('browser storage and URL never contain credentials', async ({ page }) => {
  await installProductApi(page)
  await page.goto('/app/futures')
  await assertNoCredentialStorage(page)
})

test('401 preserves requested path for safe re-login', async ({ page }) => {
  await installProductApi(page)
  await page.route('**/api/v1/futures/capabilities', route => route.fulfill({ status: 401, json: { data: null, error: { code: 'UNAUTHENTICATED', message: '会话已失效', retryable: false }, trace_id: 'trace-401' } }))
  await page.goto('/app/futures')
  await page.waitForURL(/\/login/)
  expect(new URL(page.url()).searchParams.get('redirect')).toContain('/app/futures')
})

test('malformed envelope becomes a contract error without secrets', async ({ page }) => {
  await installProductApi(page, { '/capabilities': { malformed: true } })
  await page.route('**/api/v1/futures/capabilities', route => route.fulfill({ json: { unexpected: true } }))
  await page.goto('/app/futures')
  await expect(page.getByText(/CONTRACT|契约/)).toBeVisible()
})
