import { test, expect } from '@playwright/test'
import { wrap } from './fixtures/research.js'

// B-S-01 受控 UI 验收：二级导航、列表/详情、筛选、复制深链、空态/失败重试/下架、
// 手机滚动到页尾。API 为受控替身（controlled），真链路由 Go 集成测试覆盖。

function seedSession(page) {
  return page.addInitScript(() => {
    localStorage.setItem('zhigu_token', 'e2e-token')
    localStorage.setItem('zhigu_user', 'invitee')
    localStorage.setItem('zhigu_role', 'invitee')
  })
}

const card = (over = {}) => ({
  id: 'smi_e2e',
  market_version_id: 'smv_e2e',
  version_no: 1,
  name: '均线趋势策略',
  summary: '公开均线规则整理，供学习研究使用',
  category: '趋势',
  tags: ['趋势'],
  markets: ['A'],
  signal_period: '1d',
  validation_status: 'passed',
  backtest_status: 'not_tested',
  published_at: '2026-09-20T00:00:00Z',
  updated_at: '2026-09-20T00:00:00Z',
  copyable: true,
  ...over
})

const detail = {
  ...card(),
  description: '短均线上穿长均线买入。',
  hypothesis: '假定趋势延续。',
  failure_cases: '震荡市失效。',
  sources: [{ title: '公开资料', url: 'https://example.com/a', collected_at: '2026-09-20', adaptation: '平台改编' }],
  rights_note: '仅限学习研究展示',
  editor_schema_version: 'strategy.editor.v1',
  editor_state: {
    name: '均线趋势策略',
    instrument_id: null,
    signal_period: '1d',
    price_basis: 'raw',
    indicators: [{ id: 'kdj', type: 'KDJ', params: { n: 9, m1: 3, m2: 3 } }],
    entry: { all: [{ op: 'lt', left: 'kdj.j', right: { constant: '20' } }, { kind: 'volume_increase', days: 3 }] },
    exit: { op: 'gt', left: 'kdj.j', right: { constant: '80' } },
    position: { type: 'equity_fraction', value: '0.5' },
    risk: { check: 'close', stop_loss_pct: '0.05', take_profit_pct: null, max_holding_bars: null },
    execution: { timing: 'next_session_open', priority: 'exit_first' }
  },
  backtest_defaults: { initial_cash: '100000', currency: 'CNY' },
  evidence_summaries: []
}

function mockMarket(page, { items, detailStatus = 200, detailBody = null } = {}) {
  return page.route('**/api/finance/strategy-market/**', async (route) => {
    const url = new URL(route.request().url())
    const fulfill = (body, status = 200) =>
      route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
    if (route.request().method() === 'POST' && url.pathname.endsWith('/copies')) {
      return fulfill(
        wrap({
          draft_id: 'sdr_e2e',
          revision: 1,
          status: 'needs_clarification',
          origin: { market_item_id: 'smi_e2e', market_version_id: 'smv_e2e', version_no: 1, market_status: 'published' },
          next_path: '/app/strategies?draft_id=sdr_e2e'
        }),
        201
      )
    }
    if (/\/items\/[^/]+$/.test(url.pathname)) {
      if (detailStatus !== 200) {
        return fulfill({ error: { code: detailStatus === 410 ? 'MARKET_ITEM_WITHDRAWN' : 'NOT_FOUND', message: detailStatus === 410 ? '条目已下架' : '市场条目不存在' }, data: null }, detailStatus)
      }
      return fulfill(wrap(detailBody || detail))
    }
    return fulfill(wrap({ items: items ?? [card()], next_cursor: null }))
  })
}

test('market list detail and copy deep link', async ({ page }) => {
  await seedSession(page)
  await mockMarket(page)
  await page.goto('/app/strategies/market')
  await expect(page.getByRole('navigation', { name: '交易策略导航' })).toBeVisible()
  await expect(page.getByRole('link', { name: '策略市场' })).toBeVisible()
  await expect(page.getByRole('link', { name: '策略工作台' })).toBeVisible()
  await expect(page.getByText('均线趋势策略').first()).toBeVisible()
  await expect(page.getByText('回测证据：未回测')).toBeVisible()

  await page.getByRole('link', { name: '均线趋势策略' }).click()
  await expect(page).toHaveURL(/\/app\/strategies\/market\/smi_e2e/)
  await expect(page.getByRole('heading', { name: /思路与假设/ })).toBeVisible()
  await expect(page.getByRole('heading', { name: /买卖规则与具体参数/ })).toBeVisible()
  await expect(page.getByRole('heading', { name: /来源与使用权限/ })).toBeVisible()
  // 连续放量白话展开 + 未回测不填收益。
  await expect(page.getByText(/最近连续 3 个交易日成交量逐日增加（需 4 根比较 bar）/)).toBeVisible()

  await page.getByRole('button', { name: '复制到我的策略' }).click()
  await expect(page).toHaveURL(/draft_id=sdr_e2e/)
})

test('market empty error retry and withdrawn states', async ({ page }) => {
  await seedSession(page)
  // 真实空态 + AI 创建入口。
  await mockMarket(page, { items: [] })
  await page.goto('/app/strategies/market')
  await expect(page.getByText('市场暂时没有符合条件的策略')).toBeVisible()
  await expect(page.getByRole('link', { name: '使用 AI 创建策略' })).toBeVisible()

  // 失败显示重试，不用空态掩盖接口失败。
  await page.unroute('**/api/finance/strategy-market/**')
  await page.route('**/api/finance/strategy-market/items**', (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: { code: 'INTERNAL', message: '服务异常' }, data: null }) })
  )
  await page.reload()
  await expect(page.getByRole('button', { name: '重试' })).toBeVisible()
  await expect(page.getByText('市场暂时没有符合条件的策略')).toHaveCount(0)

  // 重试恢复。
  await page.unroute('**/api/finance/strategy-market/**')
  await mockMarket(page, { items: [card()] })
  await page.getByRole('button', { name: '重试' }).click()
  await expect(page.getByText('均线趋势策略').first()).toBeVisible()

  // 下架：410 明确提示。
  await page.unroute('**/api/finance/strategy-market/**')
  await mockMarket(page, { detailStatus: 410 })
  await page.goto('/app/strategies/market/smi_e2e')
  await expect(page.getByText('该策略已下架，停止新复制')).toBeVisible()
})

test('market mobile list scrolls to page footer', async ({ page }) => {
  await seedSession(page)
  await mockMarket(page, { items: Array.from({ length: 8 }, (_, i) => card({ id: `smi_${i}`, name: `策略 ${i + 1}` })) })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/app/strategies/market')
  await page.mouse.wheel(0, 4000)
  await expect(page.getByRole('link', { name: '策略 8' })).toBeVisible()
  await page.getByRole('link', { name: '策略工作台' }).click()
  await expect(page).toHaveURL(/\/app\/strategies$/)
})
