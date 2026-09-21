import { test, expect } from '@playwright/test'
import { wrap } from './fixtures/research.js'

async function seedSession(page) {
  await page.addInitScript(() => {
    localStorage.setItem('zhigu_token', 'e2e-token')
    localStorage.setItem('zhigu_user', 'invitee')
    localStorage.setItem('zhigu_role', 'invitee')
  })
}

test('strategy workbench search select and rule card', async ({ page }) => {
  await seedSession(page)
  await page.route('**/api/finance/**', async (route) => {
    const url = route.request().url()
    const fulfill = (body, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
    if (url.includes('/api/finance/instruments') && !url.includes('/market/instruments')) {
      return fulfill(wrap({ items: [] }))
    }
    if (url.includes('/market/indicators') && !url.includes('indicator-series')) {
      return fulfill(wrap({ items: [{ type: 'MACD', pane: 'oscillator' }, { type: 'KDJ', pane: 'oscillator' }], engine_version: 'indicators.v1' }))
    }
    if (url.includes('/market/instruments?') || /\/market\/instruments$/.test(new URL(url).pathname)) {
      return fulfill(wrap({
        items: [{ instrument_id: '00700.HK', name: '腾讯控股', exchange: 'HKEX', currency: 'HKD', asset_type: 'stock', code: '00700' }],
        catalog_version: 'cat_e2e', catalog_as_of: '2026-09-18', freshness_status: 'fixture'
      }))
    }
    if (url.includes('/market/instruments/00700.HK')) {
      return fulfill(wrap({
        instrument: { instrument_id: '00700.HK', name: '腾讯控股', exchange: 'HKEX', currency: 'HKD', asset_type: 'stock' },
        coverage: { status: 'ready' }, backtest_available: true
      }))
    }
    if (url.includes('/market/ohlcv')) {
      return fulfill(wrap({
        instrument_id: '00700.HK', name: '腾讯控股', exchange: 'HKEX', currency: 'HKD', period: '1d', adjust: 'raw',
        data_snapshot_id: 'snap_e2e',
        quality: { status: 'complete', freshness_status: 'fixture', last_complete_date: '2026-09-18', warnings: ['时效未验证'] },
        bars: [
          { time: '2026-09-16', open: '370', high: '372', low: '368', close: '371', volume: '1000', is_final: true },
          { time: '2026-09-17', open: '371', high: '375', low: '370', close: '374', volume: '1100', is_final: true },
          { time: '2026-09-18', open: '374', high: '380', low: '373', close: '378', volume: '1200', is_final: true }
        ],
        has_more: false, next_cursor: null
      }))
    }
    if (url.includes('/indicator-series')) {
      return fulfill(wrap({ series: [], times: ['2026-09-16', '2026-09-17', '2026-09-18'] }))
    }
    if (url.includes('/strategy-drafts/generate')) {
      return fulfill(wrap({ generation_id: 'sgen_e2e', draft_id: 'sdr_e2e', status: 'queued', poll_url: '/api/finance/strategy-generations/sgen_e2e' }), 202)
    }
    if (url.includes('/strategy-generations/sgen_e2e')) {
      return fulfill(wrap({
        generation_id: 'sgen_e2e', draft_id: 'sdr_e2e', status: 'ready', draft_revision: 2,
        dsl: {
          schema_version: 'strategy.v1', name: '腾讯 MACD 与 KDJ 日线策略', instrument_id: '00700.HK',
          signal_period: '1d', price_basis: 'causal_qfq',
          indicators: [{ id: 'macd', type: 'MACD', params: { fast: 12, slow: 26, signal: 9 } }, { id: 'kdj', type: 'KDJ', params: { n: 9, m1: 3, m2: 3 } }],
          entry: { all: [{ op: 'crosses_above', left: 'macd.dif', right: 'macd.dea' }, { op: 'lt', left: 'kdj.k', right: { constant: '30' } }] },
          position: { type: 'equity_fraction', value: '0.5' },
          risk: { check: 'close', stop_loss_pct: '0.05' },
          execution: { timing: 'next_session_open', priority: 'exit_first' }
        },
        assumptions: ['MACD 参数未指定，使用 12/26/9']
      }))
    }
    if (url.includes('/strategy-drafts/sdr_e2e')) {
      return fulfill(wrap({ draft_id: 'sdr_e2e', revision: 3, dsl: { schema_version: 'strategy.v1', name: '腾讯 MACD 与 KDJ 日线策略', instrument_id: '00700.HK', position: { value: '0.5' }, entry: { all: [{ op: 'lt', left: 'kdj.k', right: { constant: '20' } }] } } }))
    }
    if (url.includes('/api/finance/strategies') && route.request().method() === 'POST') {
      return fulfill(wrap({ strategy_id: 'st_e2e', version_id: 'stv_e2e', revision: 1 }))
    }
    if (url.includes('/api/finance/backtests') && route.request().method() === 'POST') {
      return fulfill(wrap({ run_id: 'bt_e2e', status: 'queued', poll_url: '/api/finance/backtests/bt_e2e' }), 202)
    }
    if (url.includes('/backtests/bt_e2e/results')) {
      return fulfill(wrap({ metrics: { total_return: '0.12', max_drawdown: '0.04', win_rate: null, rf: '0' }, equity: [], signals: [], assumptions: ['历史模拟'], result_hash: 'abc123def456' }))
    }
    if (url.includes('/backtests/bt_e2e/trades')) {
      return fulfill(wrap({ fills: [{ id: 'f1', fill_date: '2026-09-18', qty: '100', price: '378', cash_delta: '-37800' }], orders: [{ id: 'o1', submitted_date: '2026-09-18', side: 'buy', status: 'filled', reason: '' }] }))
    }
    if (url.includes('/backtests/bt_e2e')) {
      return fulfill(wrap({ run_id: 'bt_e2e', status: 'succeeded' }))
    }
    return fulfill(wrap({ items: [], revision: 0 }))
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/strategies')
  await expect(page.getByRole('heading', { name: '策略助手' })).toBeVisible()
  await page.getByLabel('搜索股票').fill('700')
  await expect(page.getByRole('option').first()).toBeVisible()
  await page.getByRole('option').first().click()
  await expect(page.getByTestId('quote-ident')).toContainText('00700.HK')
  await expect(page.getByText('时效未验证', { exact: false })).toBeVisible()
  await page.getByPlaceholder(/例如：MACD/).fill('MACD 金叉且 KDJ 的 K 小于 30 时半仓买入，MACD 死叉卖出')
  await expect(page.getByTestId('generate-rule')).toBeEnabled()
  await page.getByTestId('generate-rule').click()
  await expect(page.getByTestId('gen-status')).toContainText('ready', { timeout: 15000 })
  await expect(page.getByTestId('position-value')).toContainText('0.5')
  await expect(page.getByText('历史模拟', { exact: false }).first()).toBeVisible()
  await page.getByLabel('添加指标').selectOption('MACD')
  await page.getByLabel('KDJ K 阈值').fill('20')
  await page.getByRole('button', { name: '应用修改' }).click()
  await page.getByTestId('run-backtest').click()
  await expect(page.getByText('总收益')).toBeVisible({ timeout: 10000 })
})

test('mobile strategy uses tabs', async ({ page }) => {
  await seedSession(page)
  await page.route('**/api/finance/**', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(wrap({ items: [] })) })
  })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/app/strategies')
  await expect(page.getByRole('button', { name: '行情', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '策略', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '结果', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '策略', exact: true }).click()
  await expect(page.getByRole('heading', { name: '策略助手' })).toBeVisible()
})
