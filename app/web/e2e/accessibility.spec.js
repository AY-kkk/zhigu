import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'
import { expect, test } from '@playwright/test'
import { installProductApi } from './product-fixtures.js'

const require = createRequire(import.meta.url)
const axeSource = readFileSync(require.resolve('axe-core/axe.min.js'), 'utf8')

for (const path of ['/app/futures', '/app/futures/research/new', '/app/futures/notifications', '/admin/futures/sources']) {
  test(`axe has no critical or serious issues at ${path}`, async ({ page }) => {
    await installProductApi(page)
    await page.goto(path)
    await page.addScriptTag({ content: axeSource })
    const result = await page.evaluate(async () => axe.run(document, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa'] } }))
    const blocking = result.violations.filter(item => ['critical', 'serious'].includes(item.impact))
    expect(blocking, JSON.stringify(blocking, null, 2)).toEqual([])
  })
}

test('skip link and route heading focus are keyboard reachable', async ({ page }) => {
  await installProductApi(page)
  await page.goto('/app/futures')
  await page.keyboard.press('Tab')
  await expect(page.getByRole('link', { name: '跳到主要内容' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page.locator('#page-title')).toBeFocused()
})
