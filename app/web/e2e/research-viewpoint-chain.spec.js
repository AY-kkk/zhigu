import { expect, test } from '@playwright/test'

test('real research chain is explicitly verified outside API mocks', async ({ page }) => {
  test.skip(!process.env.ZHIGU_E2E_API, '需要真实 Go + PostgreSQL + Python 链路；集成证据由 TestResearchViewpointCrossProcessFixture 提供')
  const base = process.env.ZHIGU_E2E_API
  await page.goto(`${base}/app/research/new`)
  await expect(page.getByRole('button', { name: '解析观点' })).toBeVisible()
})
