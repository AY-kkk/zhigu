import { expect, test } from '@playwright/test'
import { installProductApi } from './product-fixtures.js'

const viewports = [
  { width: 1440, height: 900 },
  { width: 1024, height: 768 },
  { width: 390, height: 844 },
  { width: 320, height: 568 },
]

for (const viewport of viewports) {
  test(`no page-level horizontal overflow at ${viewport.width}px`, async ({ page }) => {
    await page.setViewportSize(viewport)
    await installProductApi(page)
    for (const path of ['/app/futures', '/app/futures/products/SHFE.CU', '/app/futures/notifications']) {
      await page.goto(path)
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
      expect(overflow, `${path} overflow`).toBeLessThanOrEqual(0)
    }
  })
}
