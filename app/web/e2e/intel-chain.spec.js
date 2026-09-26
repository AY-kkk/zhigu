import { test, expect } from '@playwright/test'

test.skip(!process.env.E2E_INTEL_CHAIN, 'real Vue→Go→PostgreSQL chain runs only in integration mode')

test('real Vue Go PostgreSQL demo chain', async ({ page }) => {
  test.setTimeout(60000)
  await page.goto('/app/intel/demo')
  await expect(page.getByRole('button', { name: '开始演示' })).toBeVisible()
  await page.getByRole('button', { name: '开始演示' }).click()
  await expect(page.getByRole('heading', { name: '谁最先说，事实怎么变，哪些说法冲突，现在是什么状态？' })).toBeVisible()
  await page.getByRole('button', { name: '关注设置' }).click()
  await expect(page.getByText('DEMO.A').first()).toBeVisible()
  await page.getByRole('button', { name: '添加' }).nth(0).click()
  await page.getByRole('button', { name: '添加' }).nth(1).click()
  await page.getByRole('button', { name: '事件流', exact: true }).click()
  for (let i = 0; i < 3; i += 1) {
    await page.getByRole('button', { name: '下一步' }).click()
    await page.waitForTimeout(300)
  }
  await expect(page.getByText('DEMO.A 收购 DEMO.B')).toBeVisible()
  await page.getByText('DEMO.A 收购 DEMO.B').click()
  await expect(page.getByText('DEMO.A 存在收购 DEMO.B 的该项交易安排')).toBeVisible()
  await expect(page.getByText('公司正在推进收购B').first()).toBeVisible()
  await page.getByRole('button', { name: '通知中心' }).click()
  await expect(page.getByText(/固定回放步骤 S[1-3]/).first()).toBeVisible()
})
