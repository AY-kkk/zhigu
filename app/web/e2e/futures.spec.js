import { test, expect } from '@playwright/test'

const wrap = data => ({ data, error: null, trace_id: 'futures-e2e' })

async function seed(page) {
  await page.addInitScript(() => {
    localStorage.setItem('zhigu_token', 'futures-e2e-token')
    localStorage.setItem('zhigu_user', 'futures-e2e')
    localStorage.setItem('zhigu_role', 'admin')
  })
}

async function mockFutures(page, handlers = {}) {
  await page.route('**/api/v1/futures/**', async route => {
    const url = new URL(route.request().url())
    const method = route.request().method()
    const fulfill = (status, body) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
    if (url.pathname === '/api/v1/futures/capabilities') return fulfill(200, wrap(handlers.capabilities || {
      schema_version: 'futures.capabilities.v1', enabled: true, ready: true, mode: 'read_only',
      reason: 'FUTURES_E2E_FIXTURE', products: ['SHFE.CU'], features: { research: true, tracking: true, export: true },
      limits: { daily_runs: 5, active_runs: 1, active_hypotheses: 20, model_calls: 8, tool_calls: 24, tokens: 48000, user_daily_cny: '5', module_daily_cny: '50', queue_timeout_seconds: 60, execution_timeout_seconds: 180 }
    }))
    if (method === 'POST' && url.pathname === '/api/v1/futures/drafts') return fulfill(201, wrap({ id: 'draft_1', revision: 1, input: await route.request().json(), parse_state: 'not_started', parsed_revision: null, claims: [], claims_overflow: false, consumed_model_calls: 0, consumed_tokens: 0, created_at: '2026-10-05T00:00:00Z' }))
    if (method === 'GET' && url.pathname === '/api/v1/futures/products/SHFE.CU/contracts') return fulfill(200, wrap({ items: [{ id: 'CU2610', product_id: 'SHFE.CU', kind: 'actual', last_trading_at: '2026-10-15T00:00:00Z', price_precision: 0, price_unit: 'CNY/tonne', multiplier: '5', calendar_version: 'cal-v1', source_version: '1' }], next_cursor: null }))
    if (method === 'POST' && url.pathname.endsWith('/parse')) return fulfill(202, wrap({ id: 'draft_1', revision: 1, parse_state: 'succeeded', parsed_revision: 1, claims: [{ id: 'claim_1', kind: 'fact', text: '库存连续两个发布期下降', locator: null }], claims_overflow: false, consumed_model_calls: 1, consumed_tokens: 100, created_at: '2026-10-05T00:00:00Z' }))
    if (method === 'POST' && url.pathname === '/api/v1/futures/runs') return fulfill(202, wrap({ id: 'run_1', scope: { domain: 'futures', mode: 'live', owner_id: 1 }, draft_id: 'draft_1', draft_revision: 1, status: 'queued', stage: 'queued', as_of: '2026-10-05T00:00:00Z', horizon_end: '2026-10-19T00:00:00Z', report: null, failure_code: null, created_at: '2026-10-05T00:00:00Z' }))
    if (url.pathname === '/api/v1/futures/runs/run_1') return fulfill(200, wrap(handlers.run || { id: 'run_1', status: 'queued', stage: 'queued', as_of: '2026-10-05T00:00:00Z', report: null, failure_code: null, created_at: '2026-10-05T00:00:00Z' }))
    if (url.pathname === '/api/v1/futures/notifications') return fulfill(200, wrap({ items: [], next_cursor: null }))
    if (url.pathname === '/api/v1/futures/hypotheses') return fulfill(200, wrap({ items: [], next_cursor: null }))
    return fulfill(200, wrap({ items: [], next_cursor: null }))
  })
}

test('futures is the fourth business tab and off mode does not start research', async ({ page }) => {
  await seed(page)
  await mockFutures(page, { capabilities: { enabled: true, ready: false, mode: 'off', reason: 'FUTURES_MIGRATION_FAILED', products: [], features: { research: false, tracking: false, export: false }, limits: { daily_runs: 5, active_runs: 1, active_hypotheses: 20, model_calls: 8, tool_calls: 24, tokens: 48000, user_daily_cny: '5', module_daily_cny: '50', queue_timeout_seconds: 60, execution_timeout_seconds: 180 } } })
  await page.goto('/app/futures')
  const labels = await page.locator('.zhigu-nav-item').allTextContents()
  expect(labels.map(item => item.trim())).toEqual(['个人界面', '投研观点', '交易策略', '事件情报', '期货研究'])
  await expect(page.getByText('未开放')).toBeVisible()
})

test('research form freezes revision and navigates to pollable run', async ({ page }) => {
  await seed(page)
  await mockFutures(page)
  await page.goto('/app/futures/research/new')
  await page.getByLabel('研究合约').selectOption('CU2610')
  await page.getByPlaceholder('写下待验证的条件性观点').fill('如果交易所库存连续下降且现货同步走强，基差可能获得支撑。')
  await page.getByRole('button', { name: '解析观点' }).click()
  await expect(page.getByRole('button', { name: '确认并开始研究' })).toBeVisible()
  await page.getByRole('checkbox').check()
  await page.getByRole('button', { name: '确认并开始研究' }).click()
  await expect(page).toHaveURL(/\/app\/futures\/research\/run_1/)
  await expect(page.getByText('queued')).toBeVisible()
})

test('futures pages have no horizontal overflow at 390px', async ({ page }) => {
  await seed(page)
  await mockFutures(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/app/futures')
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth + 2)
  expect(overflow).toBeFalsy()
})
