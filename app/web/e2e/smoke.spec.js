import { test, expect } from '@playwright/test'

async function seedSession(page) {
  await page.addInitScript(() => {
    localStorage.setItem('zhigu_token', 'e2e-token')
    localStorage.setItem('zhigu_user', 'invitee')
    localStorage.setItem('zhigu_role', 'invitee')
  })
}

test('login page is usable at 390px', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/login')
  await expect(page.getByRole('heading', { name: '知股受邀登录' })).toBeVisible()
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth + 2)
  expect(overflow).toBeFalsy()
})

test('unauthenticated research route redirects to login', async ({ page }) => {
  await page.goto('/app/research/new')
  await expect(page).toHaveURL(/\/login/)
})

test('legacy history query still reaches history page', async ({ page }) => {
  await seedSession(page)
  await page.route('**/api/finance/**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ error: null, data: { items: [], next_cursor: '' } })
    })
  })
  await page.goto('/app/history')
  await expect(page).toHaveURL(/\/app\/history/)
})
