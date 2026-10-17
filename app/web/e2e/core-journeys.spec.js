import { expect, test } from '@playwright/test'
import { installProductApi } from './product-fixtures.js'

const routes = [
  ['/app/futures', '期货研究概览'],
  ['/app/futures/products/SHFE.CU', '铜'],
  ['/app/futures/research/new', '发起期货研究'],
  ['/app/futures/notifications', '期货通知'],
  ['/admin/futures/sources', '期货来源管理'],
]

test.describe('core product routes', () => {
  for (const [path, heading] of routes) {
    test(`renders recoverable content at ${path}`, async ({ page }) => {
      await installProductApi(page)
      await page.goto(path)
      await expect(page.locator('h1, h2').first()).toContainText(heading)
      await expect(page.locator('body')).not.toBeEmpty()
    })
  }

  test('unknown route recovers to workspace', async ({ page }) => {
    await installProductApi(page)
    await page.goto('/app/does-not-exist')
    await expect(page.getByRole('heading', { name: '暂时无法打开这个页面' })).toBeVisible()
    await expect(page.getByRole('link', { name: '返回工作台' })).toBeVisible()
  })
})
