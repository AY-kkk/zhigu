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

test('consumer shell shows three entries and welcome copy', async ({ page }) => {
  await seedSession(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/new')
  await expect(page.getByText('个人界面', { exact: true })).toBeVisible()
  await expect(page.getByText('投研观点', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('交易策略', { exact: true })).toBeVisible()
  await expect(page.getByText('让投资观点，经得起验证')).toBeVisible()
  await expect(page.getByRole('button', { name: '解析观点' })).toBeVisible()
  await expect(page.getByTestId('fixture-banner')).toContainText('离线样本演示')
})

test('legacy history url opens profile and strategies page is reserved', async ({ page }) => {
  await seedSession(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/history')
  await expect(page).toHaveURL(/\/app\/profile/)
  await page.goto('/app/strategies')
  await expect(page.getByText('策略研究功能正在规划中')).toBeVisible()
  await expect(page.getByRole('button', { name: '前往投研观点' })).toBeVisible()
})

test('mobile consumer layout has no page overflow', async ({ page }) => {
  await seedSession(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/app/research/new')
  await expect(page.getByRole('button', { name: '打开导航' })).toBeVisible()
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth + 2)
  expect(overflow).toBeFalsy()
})

test('login succeeds against live API', async ({ page }) => {
  test.skip(!process.env.ZHIGU_E2E_API, '需要本机 Go+Python+Postgres；安装浏览器不能代替登录成功')
  await page.goto('/login')
  await page.getByPlaceholder('用户名').fill('invitee')
  await page.getByPlaceholder('密码').fill('Passw0rd!')
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForURL(/\/app\/research\/new/)
})

test('login-parse-confirm-cancel loop requires live API', async ({ page }) => {
  test.skip(!process.env.ZHIGU_E2E_API, '需要本机 Go+Python+Postgres；安装浏览器不能代替该闭环')
  await page.goto('/login')
  await page.getByPlaceholder('用户名').fill('invitee')
  await page.getByPlaceholder('密码').fill('Passw0rd!')
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForURL(/\/app\/research\/new/)
  await page.getByRole('textbox').fill('演示公司的收入增长能否支持未来一年股价上涨？这是一条超过二十个字的测试观点。')
  await page.getByRole('button', { name: '解析观点' }).click()
  await expect(page.getByRole('button', { name: '确认并开始研究' })).toBeVisible({ timeout: 15000 })
  await page.getByRole('button', { name: '确认并开始研究' }).click()
  await page.waitForURL(/\/app\/research\/run_/)
  await expect(page.getByText(/排队|调查|核对|完成|未完成|失败|取消/)).toBeVisible({ timeout: 30000 })
  const cancel = page.getByRole('button', { name: '取消研究' })
  if (await cancel.isVisible()) {
    await cancel.click()
    await expect(page.getByText(/取消中|已取消|失败|未完成/)).toBeVisible({ timeout: 15000 })
  }
})

test('edited claim disables confirm until re-parse', async ({ page }) => {
  test.skip(!process.env.ZHIGU_E2E_API, '需要本机 Go+Python+Postgres')
  await page.goto('/login')
  await page.getByPlaceholder('用户名').fill('invitee')
  await page.getByPlaceholder('密码').fill('Passw0rd!')
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForURL(/\/app\/research\/new/)
  await page.getByRole('textbox').fill('演示公司的收入增长能否支持未来一年股价上涨？这是一条超过二十个字的测试观点。')
  await page.getByRole('button', { name: '解析观点' }).click()
  await expect(page.getByRole('button', { name: '确认并开始研究' })).toBeVisible({ timeout: 15000 })
  await page.getByRole('textbox').fill('经营现金流持续下降，这是否足以否定此前收入增长支持股价的判断？')
  await expect(page.getByRole('button', { name: '确认并开始研究' })).toHaveCount(0)
})
