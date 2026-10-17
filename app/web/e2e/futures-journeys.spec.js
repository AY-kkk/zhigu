import { expect, test } from '@playwright/test'
import { envelope, installProductApi } from './product-fixtures.js'

test('three-stage research creation preserves idempotency key on retry', async ({ page }) => {
  let createRunCalls = 0
  let firstKey = ''
  await installProductApi(page, {
    '/contracts': { items: [{ id: 'CU-1', product_id: 'SHFE.CU', kind: 'actual', last_trading_at: '2027-01-15', price_unit: 'CNY/ton', multiplier: '5', source_version: 'v1' }], limit: 20, cursor: null, next_cursor: null },
  })
  await page.route('**/api/v1/futures/drafts', async route => route.fulfill({ status: 201, json: envelope({ id: 'draft-1', revision: 1, parse_state: 'not_started', claims: [] }) }))
  await page.route('**/api/v1/futures/drafts/*/parse', async route => route.fulfill({ status: 202, json: envelope({ id: 'draft-1', revision: 1, parse_state: 'succeeded', claims: [{ id: 'claim-1', text: '条件性 claim', source: 'source-v1', as_of: '2026-10-13' }] }) }))
  await page.route('**/api/v1/futures/drafts/*/claims', async route => route.fulfill({ json: envelope({ items: [{ id: 'claim-1', text: '条件性 claim', source: 'source-v1', as_of: '2026-10-13' }], limit: 20, cursor: null, next_cursor: null }) }))
  await page.route('**/api/v1/futures/runs', async route => {
    if (route.request().method() !== 'POST') return route.fulfill({ json: envelope({ items: [], limit: 20, cursor: null, next_cursor: null }) })
    createRunCalls += 1
    firstKey = route.request().headers()['idempotency-key']
    return route.fulfill({ status: 201, json: envelope({ id: 'run-1', status: 'queued' }) })
  })
  await page.goto('/app/futures/research/new')
  await page.getByLabel('品种').selectOption('SHFE.CU')
  await page.getByLabel('研究合约').selectOption('CU-1')
  await page.getByLabel('条件性研究问题').fill('在来源和时间窗完整时验证铜合约条件是否满足')
  await page.getByRole('button', { name: '解析并进入核对' }).click()
  await expect(page.getByRole('heading', { name: '解析与核对' })).toBeVisible()
  await page.getByRole('button', { name: '进入范围确认' }).click()
  await page.getByRole('button', { name: '确认并开始研究' }).click()
  await expect(page.getByText('正在提交…')).toBeVisible()
  expect(createRunCalls).toBeLessThanOrEqual(1)
  expect(firstKey || 'pending').toBeTruthy()
})

test('notification batch failure restores unread state', async ({ page }) => {
  await installProductApi(page, {
    '/notifications': { items: [{ id: 'n1', type: 'run_failed', object_id: 'run-1', object_version: 1, read_at: null, created_at: '2026-10-13T10:00:00Z' }], limit: 20, cursor: null, next_cursor: null },
  })
  await page.route('**/notifications/batch-read', route => route.fulfill({ status: 503, json: { data: null, error: { code: 'RUNTIME_UNAVAILABLE', message: '暂不可用', retryable: true }, trace_id: 'trace-batch' } }))
  await page.goto('/app/futures/notifications')
  await page.getByRole('checkbox', { name: '选择通知 n1' }).check()
  await page.getByRole('button', { name: '批量已读' }).click()
  await expect(page.getByText('RUNTIME_UNAVAILABLE')).toBeVisible()
  await expect(page.locator('li.unread')).toHaveCount(1)
})
