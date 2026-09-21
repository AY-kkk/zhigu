import { test, expect } from '@playwright/test'
import {
  DEMO_CLAIM,
  evidenceDoc,
  errorBody,
  historyItem,
  historyPage,
  parseSuccess,
  report,
  runView,
  wrap
} from './fixtures/research.js'

function claimInput(page) {
  return page.getByPlaceholder(/粘贴一个关于单家 A 股或港股公司的投资观点/)
}

function followupInput(page) {
  return page.getByPlaceholder(/基于这份报告追问/)
}

async function seedSession(page) {
  await page.addInitScript(() => {
    localStorage.setItem('zhigu_token', 'e2e-token')
    localStorage.setItem('zhigu_user', 'invitee')
    localStorage.setItem('zhigu_role', 'invitee')
  })
}

async function mockApi(page, handlers = {}) {
  await page.route('**/api/finance/**', async (route) => {
    const req = route.request()
    const url = req.url()
    const method = req.method()
    const fulfill = (status, body) => route.fulfill({
      status,
      contentType: 'application/json',
      body: JSON.stringify(body)
    })
    if (url.includes('/api/finance/instruments')) return fulfill(200, wrap({ items: [{ instrument_id: 'DEMO:COMPANY', symbol: 'DEMO', name: '演示公司', market: 'A' }], mode: 'fixture' }))
    if (method === 'POST' && url.includes('/api/finance/claims/parse')) {
      if (handlers.parse) return handlers.parse(route, fulfill)
      return fulfill(200, parseSuccess())
    }
    if (method === 'PATCH' && url.includes('/api/finance/claims/')) {
      return fulfill(200, parseSuccess())
    }
    if (method === 'POST' && /\/api\/finance\/research$/.test(new URL(url).pathname)) {
      if (handlers.create) return handlers.create(route, fulfill)
      return fulfill(200, wrap({ run_id: 'run_e2e_1', status: 'queued', poll_url: '/api/finance/research/run_e2e_1' }))
    }
    if (method === 'GET' && /\/api\/finance\/research\?/.test(url) || (method === 'GET' && /\/api\/finance\/research$/.test(new URL(url).pathname))) {
      if (handlers.list) return handlers.list(route, fulfill)
      return fulfill(200, historyPage([]))
    }
    if (method === 'GET' && url.includes('/api/finance/research/')) {
      if (handlers.get) return handlers.get(route, fulfill, url)
      return fulfill(200, runView({ status: 'researching', report: null }))
    }
    if (method === 'GET' && url.includes('/api/finance/evidence/')) {
      if (handlers.evidence) return handlers.evidence(route, fulfill, url)
      return fulfill(200, evidenceDoc())
    }
    if (method === 'POST' && url.includes('/questions')) {
      if (handlers.question) return handlers.question(route, fulfill)
      return fulfill(200, wrap({
        answer: '仅基于本报告已引用的证据作答。',
        claim_type: 'assumption',
        evidence_ids: ['ev_1'],
        limitations: ['追问不调用外部工具。'],
        mode: 'fixture'
      }))
    }
    if (method === 'DELETE' && url.includes('/api/finance/research/')) {
      return fulfill(202, wrap({ deletion_status: 'scheduled' }))
    }
    if (method === 'POST' && url.includes('/cancel')) {
      return fulfill(200, wrap({ status: 'canceling' }))
    }
    return fulfill(404, errorBody(404, 'NOT_FOUND', 'not found'))
  })
}

test('login page is usable at 390px', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/login')
  await expect(page.getByRole('heading', { name: '知股受邀登录' })).toBeVisible()
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth + 2)
  expect(overflow).toBeFalsy()
})

test('typing keeps the welcome input stable and the brand artwork available', async ({ page }) => {
  await seedSession(page)
  await mockApi(page)
  await page.goto('/app/research/new')
  const input = claimInput(page)
  const before = await input.boundingBox()
  await input.fill(DEMO_CLAIM)
  await expect(page.getByRole('heading', { name: '让投资观点，经得起验证' })).toBeVisible()
  const after = await input.boundingBox()
  expect(Math.abs(after.y - before.y)).toBeLessThan(2)
  await expect(input).toBeFocused()
  const mark = page.locator('.zg-logo-mark use').first()
  const href = await mark.getAttribute('href')
  expect(href).not.toMatch(/^data:/)
  const asset = await page.request.get(href.split('#')[0])
  expect(asset.ok()).toBeTruthy()
  expect(await asset.text()).toContain('id="zhigu-mark"')
})

test('fact tags do not imply support inside a challenge argument', async ({ page }) => {
  await seedSession(page)
  await mockApi(page, { get: (_route, fulfill) => fulfill(200, runView({ report: report('challenged') })) })
  await page.goto('/app/research/run_e2e_1')
  const challenge = page.locator('#zg-challenge .arg').first()
  const typeTag = challenge.locator('.tag').first()
  const polarityTag = challenge.locator('.tag').nth(1)
  expect(await typeTag.evaluate(el => getComputedStyle(el).color)).toBe('rgb(102, 102, 102)')
  expect(await polarityTag.evaluate(el => getComputedStyle(el).color)).toBe('rgb(46, 125, 50)')
})

test('unauthenticated research route redirects to login', async ({ page }) => {
  await page.goto('/app/research/new')
  await expect(page).toHaveURL(/\/login/)
})

test('consumer shell shows three entries and welcome copy', async ({ page }) => {
  await seedSession(page)
  await mockApi(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/new')
  await expect(page.getByText('个人界面', { exact: true })).toBeVisible()
  await expect(page.getByText('投研观点', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('交易策略', { exact: true })).toBeVisible()
  await expect(page.getByText('让投资观点，经得起验证')).toBeVisible()
  await expect(page.getByRole('button', { name: '解析观点' })).toBeVisible()
  await expect(page.getByTestId('fixture-banner')).toContainText('离线样本演示')
})

test('history url stays on history and strategies page is reserved', async ({ page }) => {
  await seedSession(page)
  await mockApi(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/history')
  await expect(page).toHaveURL(/\/app\/history/)
  await expect(page.getByRole('heading', { name: '我的研究' })).toBeVisible()
  await page.goto('/app/profile?tab=history&keep=1')
  await expect(page).toHaveURL(/\/app\/history/)
  await expect(page).toHaveURL(/keep=1/)
  await page.goto('/app/strategies')
  await expect(page.getByRole('search')).toBeVisible()
  await expect(page.getByText('策略助手')).toBeVisible()
  await expect(page.getByRole('button', { name: '运行回测' })).toBeVisible()
})

test('mobile consumer layout has no page overflow and nav is usable', async ({ page }) => {
  await seedSession(page)
  await mockApi(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/app/research/new')
  await expect(page.getByRole('button', { name: '打开导航' })).toBeVisible()
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth + 2)
  expect(overflow).toBeFalsy()
  await page.getByRole('button', { name: '打开导航' }).click()
  const nav = page.getByRole('dialog')
  await expect(nav.getByRole('button', { name: '个人界面' })).toBeInViewport()
  await expect(nav.getByRole('button', { name: '投研观点' })).toBeInViewport()
  await expect(nav.getByRole('button', { name: '交易策略' })).toBeInViewport()
  const box = await nav.getByRole('button', { name: '个人界面' }).boundingBox()
  expect(box).toBeTruthy()
  expect(box.x).toBeGreaterThanOrEqual(-1)
  expect(box.y + box.height).toBeLessThan(844)
  expect(box.x + box.width).toBeLessThanOrEqual(390)
  await nav.getByRole('button', { name: '交易策略' }).click()
  await expect(page).toHaveURL(/\/app\/strategies/)
})

test('claim length 0 stays unerrorred until submit; 2000 is valid', async ({ page }) => {
  await seedSession(page)
  await mockApi(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/new')
  const box = claimInput(page)
  await expect(page.getByText('0/2000')).toBeVisible()
  await expect(page.getByText('请再写')).toHaveCount(0)
  await box.fill('短')
  await box.blur()
  await expect(page.getByText(/至少 20 字/)).toBeVisible()
  await box.fill(DEMO_CLAIM)
  await expect(page.getByRole('button', { name: '解析观点' })).toBeEnabled()
})

test('parse confirm start uses mocked API and does not auto create', async ({ page }) => {
  let created = 0
  await seedSession(page)
  await mockApi(page, {
    create: async (_route, fulfill) => {
      created += 1
      return fulfill(200, wrap({ run_id: 'run_e2e_1', status: 'queued' }))
    },
    get: async (_route, fulfill) => fulfill(200, runView({ status: 'queued', report: null }))
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/new')
  await claimInput(page).fill(DEMO_CLAIM)
  await page.getByRole('button', { name: '解析观点' }).click()
  await expect(page.getByRole('button', { name: '确认并开始研究' })).toBeVisible()
  expect(created).toBe(0)
  await page.getByRole('button', { name: '确认并开始研究' }).click()
  await expect(page).toHaveURL(/\/app\/research\/run_e2e_1/, { timeout: 10000 })
  await expect(page.getByText('正在排队等待研究')).toBeVisible()
  expect(created).toBe(1)
})

test('verdict colors and incomplete do not invent a total judgment', async ({ page }) => {
  await seedSession(page)
  await mockApi(page, {
    get: async (_route, fulfill, url) => {
      if (url.includes('run_supported')) return fulfill(200, runView({ run_id: 'run_supported', report: report('supported', { run_id: 'run_supported' }) }))
      if (url.includes('run_challenged')) return fulfill(200, runView({ run_id: 'run_challenged', report: report('challenged', { run_id: 'run_challenged' }) }))
      if (url.includes('run_incomplete')) {
        return fulfill(200, runView({
          run_id: 'run_incomplete',
          status: 'incomplete',
          report: report(null, { run_id: 'run_incomplete', quality_status: 'incomplete', verdict: null })
        }))
      }
      return fulfill(200, runView())
    }
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/run_supported')
  await expect(page.getByRole('heading', { name: '得到支持' })).toBeVisible()
  await page.goto('/app/research/run_challenged')
  await expect(page.getByRole('heading', { name: '受到挑战' })).toBeVisible()
  await page.goto('/app/research/run_incomplete')
  await expect(page.getByText('研究未完成', { exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: '得到支持' })).toHaveCount(0)
})

test('evidence drawer is in viewport and retry uses requested id', async ({ page }) => {
  const requested = []
  let failOnce = true
  await seedSession(page)
  await mockApi(page, {
    get: async (_route, fulfill) => fulfill(200, runView({ report: report('supported') })),
    evidence: async (_route, fulfill, url) => {
      const id = url.split('/evidence/')[1]
      requested.push(id)
      if (failOnce) {
        failOnce = false
        return fulfill(500, errorBody(500, 'INTERNAL', 'boom'))
      }
      return fulfill(200, evidenceDoc(id))
    }
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/run_e2e_1')
  await page.getByRole('button', { name: /查看证据 1/ }).first().click()
  await expect(page.getByText('证据加载失败')).toBeVisible()
  const drawer = page.locator('.semi-sidesheet, [aria-label="证据详情"]').first()
  await expect(drawer).toBeVisible()
  const retry = page.getByRole('button', { name: '重试加载' })
  await expect(retry).toBeVisible()
  await expect(retry).toBeInViewport()
  await retry.click()
  await expect(page.getByText('演示公司年度报告摘要')).toBeVisible()
  await page.getByRole('tab', { name: '相关数据' }).click()
  await expect(page.getByRole('cell', { name: '0', exact: true })).toBeVisible()
  expect(requested[0]).toBe('ev_1')
  expect(requested[1]).toBe('ev_1')
})

test('short followup can send and question limit is recoverable', async ({ page }) => {
  await seedSession(page)
  await mockApi(page, {
    get: async (_route, fulfill) => fulfill(200, runView({ report: report('mixed') })),
    question: async (_route, fulfill) => fulfill(429, errorBody(429, 'QUESTION_LIMIT', 'limit'))
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/run_e2e_1')
  await expect(page.getByRole('heading', { name: '证据混合' })).toBeVisible()
  await expect(followupInput(page)).toHaveValue('')
  await followupInput(page).fill('依据是什么？')
  await page.getByRole('button', { name: '发送追问' }).click()
  await expect(page.getByText('已达到每份报告 3 轮追问上限，请发起重新研究')).toBeVisible()
  await expect(page.getByRole('button', { name: '重新研究' })).toBeVisible()
})

test('history empty error and pagination stay distinct; delete is scheduled', async ({ page }) => {
  let listMode = 'empty'
  await seedSession(page)
  await mockApi(page, {
    list: async (_route, fulfill) => {
      if (listMode === 'fail') return fulfill(500, errorBody(500, 'INTERNAL', 'boom'))
      if (listMode === 'one') return fulfill(200, historyPage([historyItem({ deleting: undefined })]))
      return fulfill(200, historyPage([]))
    }
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/history')
  await expect(page.getByText('还没有研究记录')).toBeVisible()
  listMode = 'fail'
  await page.reload()
  await expect(page.getByText('无法加载研究记录')).toBeVisible()
  await expect(page.getByText('还没有研究记录')).toHaveCount(0)
  listMode = 'one'
  await page.getByRole('button', { name: '重试' }).click()
  await expect(page.getByText(/观点研究/)).toBeVisible()
})

test('cross-run late response does not paint the previous report', async ({ page }) => {
  await seedSession(page)
  await mockApi(page, {
    get: async (route, fulfill, url) => {
      if (url.includes('run_slow')) {
        await new Promise((r) => setTimeout(r, 800))
        return fulfill(200, runView({ run_id: 'run_slow', report: report('supported', { run_id: 'run_slow', summary: '慢请求的支持结论' }) }))
      }
      return fulfill(200, runView({ run_id: 'run_fast', report: report('challenged', { run_id: 'run_fast', summary: '快请求的挑战结论' }) }))
    }
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/run_slow')
  await page.goto('/app/research/run_fast')
  await expect(page.getByText('快请求的挑战结论')).toBeVisible()
  await page.waitForTimeout(1000)
  await expect(page.getByText('慢请求的支持结论')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '受到挑战' })).toBeVisible()
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
  await seedSession(page)
  await mockApi(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/new')
  await claimInput(page).fill(DEMO_CLAIM)
  await page.getByRole('button', { name: '解析观点' }).click()
  await expect(page.getByRole('button', { name: '确认并开始研究' })).toBeVisible()
  await claimInput(page).fill('经营现金流持续下降，这是否足以否定此前收入增长支持股价的判断？')
  await expect(page.getByRole('button', { name: '确认并开始研究' })).toHaveCount(0)
})
