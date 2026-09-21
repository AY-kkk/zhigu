import { expect, test } from '@playwright/test'
import { DEMO_CLAIM, evidenceDoc, historyItem, historyPage, parseSuccess, report, runView, wrap } from './fixtures/research.js'

const out = '../../reviews/editorial-implementation/run-1'

async function seedSession(page) {
  await page.addInitScript(() => {
    localStorage.setItem('zhigu_token', 'e2e-token')
    localStorage.setItem('zhigu_user', 'invitee')
    localStorage.setItem('zhigu_role', 'invitee')
  })
}

async function mockApi(page) {
  await page.route('**/api/finance/**', async (route) => {
    const url = route.request().url()
    const method = route.request().method()
    const fulfill = (body) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) })
    if (url.includes('/instruments')) return fulfill(wrap({ items: [{ instrument_id: 'DEMO:COMPANY', symbol: 'DEMO', name: '演示公司', market: 'A' }], mode: 'fixture' }))
    if (url.includes('/evidence/')) return fulfill(evidenceDoc(url.split('/evidence/')[1]))
    if (method === 'POST' && url.includes('/claims/parse')) return fulfill(parseSuccess())
    if (method === 'PATCH' && url.includes('/claims/')) return fulfill(parseSuccess())
    if (method === 'GET' && /\/research\?/.test(url) || (method === 'GET' && /\/research$/.test(new URL(url).pathname))) {
      return fulfill(historyPage([historyItem()]))
    }
    if (url.includes('/research/run_challenged')) return fulfill(runView({ run_id: 'run_challenged', report: report('challenged', { run_id: 'run_challenged' }) }))
    if (url.includes('/research/run_mixed')) return fulfill(runView({ run_id: 'run_mixed', report: report('mixed', { run_id: 'run_mixed' }) }))
    if (url.includes('/research/run_insufficient')) return fulfill(runView({ run_id: 'run_insufficient', report: report('insufficient', { run_id: 'run_insufficient' }) }))
    if (url.includes('/research/run_incomplete')) {
      return fulfill(runView({
        run_id: 'run_incomplete',
        status: 'incomplete',
        report: report(null, { run_id: 'run_incomplete', quality_status: 'incomplete', verdict: null })
      }))
    }
    if (url.includes('/research/run_researching')) return fulfill(runView({ status: 'researching', report: null }))
    if (url.includes('/research/')) return fulfill(runView({ report: report('supported') }))
    return fulfill(wrap({}))
  })
}

const views = [
  [1440, 900],
  [1024, 768],
  [768, 1024],
  [390, 844]
]

test('review screenshots', async ({ page }) => {
  test.skip(!process.env.ZHIGU_SHOTS, '本地审查截图；不作为默认 e2e 门禁')
  test.setTimeout(120000)
  await seedSession(page)
  await mockApi(page)

  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/login')
  await expect(page.getByRole('heading', { name: '知股受邀登录' })).toBeVisible()
  await page.screenshot({ path: `${out}/R1-login-1440x900.png`, fullPage: false })

  for (const [w, h] of views) {
    await page.setViewportSize({ width: w, height: h })
    await page.goto('/app/research/new')
    await expect(page.getByRole('heading', { name: '让投资观点，经得起验证' })).toBeVisible()
    await page.screenshot({ path: `${out}/R2-new-pristine-${w}x${h}.png`, fullPage: false })

    await page.goto('/app/research/run_researching')
    await expect(page.getByText('正在调查支持与反证')).toBeVisible()
    await page.screenshot({ path: `${out}/R3-researching-${w}x${h}.png`, fullPage: false })

    await page.goto('/app/research/run_e2e_1')
    await expect(page.getByRole('heading', { name: '得到支持' })).toBeVisible()
    await page.screenshot({ path: `${out}/R4-report-supported-${w}x${h}.png`, fullPage: false })

    await page.goto('/app/history')
    await expect(page.getByRole('heading', { name: '我的研究' })).toBeVisible()
    await page.screenshot({ path: `${out}/R5-history-${w}x${h}.png`, fullPage: false })
  }

  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/run_challenged')
  await expect(page.getByRole('heading', { name: '受到挑战' })).toBeVisible()
  await page.screenshot({ path: `${out}/R4-report-challenged-1440x900.png`, fullPage: false })
  await page.goto('/app/research/run_mixed')
  await expect(page.getByRole('heading', { name: '证据混合' })).toBeVisible()
  await page.screenshot({ path: `${out}/R4-report-mixed-1440x900.png`, fullPage: false })
  await page.goto('/app/research/run_insufficient')
  await expect(page.getByRole('heading', { name: '证据不足' })).toBeVisible()
  await page.screenshot({ path: `${out}/R4-report-insufficient-1440x900.png`, fullPage: false })
  await page.goto('/app/research/run_incomplete')
  await expect(page.getByText('研究未完成', { exact: true })).toBeVisible()
  await page.screenshot({ path: `${out}/R4-report-incomplete-1440x900.png`, fullPage: false })

  await page.goto('/app/research/new')
  await page.getByPlaceholder(/粘贴一个关于单家 A 股或港股公司的投资观点/).fill(DEMO_CLAIM)
  await page.getByRole('button', { name: '解析观点' }).click()
  await expect(page.getByRole('button', { name: '确认并开始研究' })).toBeVisible()
  await page.screenshot({ path: `${out}/R2-confirm-1440x900.png`, fullPage: false })

  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/app/research/new')
  await expect(page.getByRole('button', { name: '打开导航' })).toBeVisible()
  await page.getByRole('button', { name: '打开导航' }).click()
  const nav = page.getByRole('dialog')
  await expect(nav.getByRole('button', { name: '个人界面' })).toBeInViewport()
  await expect(nav.getByRole('button', { name: '交易策略' })).toBeInViewport()
  await page.screenshot({ path: `${out}/R1-mobile-nav-390x844.png`, fullPage: false })
})

async function overflowX(page) {
  return page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
}

function drawerBox(page) {
  return page.locator('.zg-evidence-drawer .semi-sidesheet-inner-wrap').first().boundingBox()
}

// 抽屉是 240ms 横向滑入；量位置前必须等它停稳。
async function settleDrawer(page, rightEdge) {
  let box = await drawerBox(page)
  for (let i = 0; i < 30 && Math.round(box.x + box.width) !== rightEdge; i += 1) {
    await page.waitForTimeout(100)
    box = await drawerBox(page)
  }
  return box
}

test('qa sweep: overlays, narrow viewport, report tail', async ({ page }) => {
  test.skip(!process.env.ZHIGU_SHOTS, '本地审查截图；不作为默认 e2e 门禁')
  test.setTimeout(120000)
  await seedSession(page)
  await mockApi(page)

  for (const [w, h] of [[1440, 900], [1024, 768], [768, 1024], [390, 844], [320, 720], [720, 450]]) {
    await page.setViewportSize({ width: w, height: h })
    await page.goto('/app/research/new')
    await expect(page.getByRole('heading', { name: '让投资观点，经得起验证' })).toBeVisible()
    expect(await overflowX(page), `new ${w}px 横向溢出`).toBeLessThanOrEqual(2)
    await page.goto('/app/research/run_e2e_1')
    await expect(page.getByRole('heading', { name: '得到支持' })).toBeVisible()
    expect(await overflowX(page), `report ${w}px 横向溢出`).toBeLessThanOrEqual(2)
  }
  await page.setViewportSize({ width: 320, height: 720 })
  await page.goto('/app/research/new')
  await expect(page.getByRole('heading', { name: '让投资观点，经得起验证' })).toBeVisible()
  await page.screenshot({ path: `${out}/R6-new-320x720.png`, fullPage: false })
  await page.setViewportSize({ width: 720, height: 450 })
  await page.goto('/app/research/run_e2e_1')
  await expect(page.getByRole('heading', { name: '得到支持' })).toBeVisible()
  await page.screenshot({ path: `${out}/R6-report-zoom200-720x450.png`, fullPage: false })

  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/run_e2e_1')
  const tail = page.getByRole('button', { name: '复制报告' })
  await tail.scrollIntoViewIfNeeded()
  await expect(tail).toBeInViewport()
  const tailBox = await tail.boundingBox()
  const composerTop = await page.evaluate(() => document.getElementById('zg-composer').getBoundingClientRect().top)
  expect(tailBox.y + tailBox.height, '报告末尾被 Composer 覆盖').toBeLessThanOrEqual(composerTop + 1)
  await page.screenshot({ path: `${out}/R4-report-tail-1440x900.png`, fullPage: false })

  await page.getByRole('button', { name: /查看证据 1/ }).first().click()
  await expect(page.getByText('演示公司年度报告摘要')).toBeVisible()
  await expect(page.getByRole('button', { name: '关闭证据详情' })).toBeVisible()
  // 抽屉是 240ms 横向滑入；断言与截图都要等它停稳，否则量到的是动画中途位置。
  const settled = await settleDrawer(page, 1440)
  expect(Math.round(settled.x + settled.width), `抽屉右边缘应贴视口: ${JSON.stringify(settled)}`).toBe(1440)
  expect(Math.round(settled.width), '1440 抽屉宽度应为 520').toBe(520)
  await page.screenshot({ path: `${out}/R4-drawer-text-1440x900.png`, fullPage: false })
  await page.getByRole('tab', { name: '相关数据' }).click()
  await expect(page.getByRole('cell', { name: '0', exact: true })).toBeVisible()
  // Tab 切换是淡入；等指标表完全不透明后再截图。
  await expect.poll(() => page.getByRole('cell', { name: '0', exact: true }).evaluate((el) => {
    let opacity = 1
    for (let node = el; node && node.nodeType === 1; node = node.parentElement) {
      opacity *= parseFloat(getComputedStyle(node).opacity || '1')
    }
    return opacity >= 0.999 ? 'opaque' : 'fading'
  })).toBe('opaque')
  await page.screenshot({ path: `${out}/R4-drawer-data-1440x900.png`, fullPage: false })
  await page.keyboard.press('Escape')

  await page.getByRole('button', { name: '更多操作' }).click()
  await page.getByText('删除研究').click()
  await expect(page.getByText('从研究记录中移除？')).toBeVisible()
  await expect(page.getByRole('button', { name: '确认删除' })).toBeVisible()
  await expect.poll(() => page.locator('.semi-modal-content').first().boundingBox().then((b) => Math.round(b.width))).toBe(448)
  await page.screenshot({ path: `${out}/R5-delete-confirm-1440x900.png`, fullPage: false })
  await page.getByRole('button', { name: '返回' }).click()
  await expect(page.getByText('从研究记录中移除？')).toHaveCount(0)

  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/app/research/run_e2e_1')
  await page.getByRole('button', { name: /查看证据 1/ }).first().click()
  await expect(page.getByText('演示公司年度报告摘要')).toBeVisible()
  const mobileBox = await settleDrawer(page, 390)
  expect(Math.round(mobileBox.x), `390 抽屉应从左边缘起: ${JSON.stringify(mobileBox)}`).toBe(0)
  expect(Math.round(mobileBox.width), '390 抽屉未全屏').toBe(390)
  await page.screenshot({ path: `${out}/R4-drawer-390x844.png`, fullPage: false })

  // 减弱动态效果：浮层入场不位移，关闭后不留遮罩。
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/run_e2e_1')
  await expect(page.getByRole('heading', { name: '得到支持' })).toBeVisible()
  await page.getByRole('button', { name: /查看证据 1/ }).first().click()
  await expect(page.getByRole('button', { name: '关闭证据详情' })).toBeVisible()
  const instant = await drawerBox(page)
  expect(Math.round(instant.x + instant.width), `减弱动态效果下抽屉应直接到位: ${JSON.stringify(instant)}`).toBe(1440)
  await page.getByRole('button', { name: '关闭证据详情' }).click()
  await expect(page.locator('.zg-evidence-drawer .semi-sidesheet-mask')).toHaveCount(0)
  await page.emulateMedia({ reducedMotion: null })
})
