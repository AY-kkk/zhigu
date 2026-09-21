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
        items: [{ instrument_id: '00700.HK', name: '腾讯控股', exchange: 'HKEX', currency: 'HKD', asset_type: 'stock', code: '00700', last: '430', change: '11', change_pct: '2.63' }],
        catalog_version: 'cat_e2e', catalog_as_of: '2026-09-18', freshness_status: 'fixture'
      }))
    }
    if (url.includes('/market/instruments/00700.HK')) {
      return fulfill(wrap({
        instrument: { instrument_id: '00700.HK', name: '腾讯控股', exchange: 'HKEX', currency: 'HKD', asset_type: 'stock' },
        coverage: { status: 'ready' }, backtest_available: true
      }))
    }
    if (url.includes('/market/quotes')) {
      return fulfill(wrap({ items: [{ instrument_id: '00700.HK', last: '430', change: '11', change_pct: '2.63' }] }))
    }
    if (url.includes('/market/quote')) {
      return fulfill(wrap({
        instrument_id: '00700.HK', name: '腾讯控股', last: '430', change: '11', change_pct: '2.63', volume: '19669333'
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
  await expect(page.getByRole('button', { name: '移除 MACD' })).toBeVisible()
  await expect(page.getByRole('button', { name: '移除 KDJ' })).toBeVisible()
  await expect(page.getByRole('button', { name: '移除 VOL' })).toBeVisible()
  await page.getByLabel('搜索股票').fill('700')
  await expect(page.getByRole('option').first()).toBeVisible()
  await expect(page.getByTestId('hit-last')).toContainText('430')
  await page.getByRole('option').first().click()
  await expect(page.getByTestId('quote-ident')).toContainText('00700.HK')
  await expect(page.getByTestId('quote-last')).toContainText('430')
  await expect(page.getByTestId('quote-date')).toHaveText('2026-09-18')
  await expect(page.getByTestId('chart-host')).toBeVisible()
  await expect(page.getByTestId('chart-reset')).toBeVisible()
  await page.getByTestId('chart-reset').click()
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

test('strategy chart OHLC panel reset and older bars', async ({ page }) => {
  await seedSession(page)
  await page.route('**/api/finance/**', async (route) => {
    const url = route.request().url()
    const fulfill = (body, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
    if (url.includes('/market/indicators') && !url.includes('indicator-series')) {
      return fulfill(wrap({ items: [{ type: 'MACD', pane: 'oscillator' }, { type: 'MA', pane: 'price' }], engine_version: 'indicators.v1' }))
    }
    if (url.includes('/market/instruments?') || /\/market\/instruments$/.test(new URL(url).pathname)) {
      return fulfill(wrap({
        items: [{ instrument_id: '600519.SH', name: '贵州茅台', exchange: 'SSE', currency: 'CNY', asset_type: 'stock', code: '600519' }],
        catalog_version: 'cat_e2e', catalog_as_of: '2026-09-18', freshness_status: 'fixture'
      }))
    }
    if (url.includes('/market/instruments/600519.SH')) {
      return fulfill(wrap({
        instrument: { instrument_id: '600519.SH', name: '贵州茅台', exchange: 'SSE', currency: 'CNY', asset_type: 'stock' },
        coverage: { status: 'ready' }, backtest_available: true
      }))
    }
    if (url.includes('/market/quotes')) {
      return fulfill(wrap({ items: [{ instrument_id: '600519.SH', last: '1252.57', change: '-4.55', change_pct: '-0.36' }] }))
    }
    if (url.includes('/market/quote')) {
      return fulfill(wrap({
        instrument_id: '600519.SH', name: '贵州茅台', last: '1252.57', change: '-4.55', change_pct: '-0.36', volume: '2501700'
      }))
    }
    if (url.includes('/market/ohlcv')) {
      const older = url.includes('cursor=')
      return fulfill(wrap({
        instrument_id: '600519.SH', name: '贵州茅台', exchange: 'SSE', currency: 'CNY', period: '1d', adjust: 'raw',
        data_snapshot_id: 'snap_e2e',
        quality: { status: 'complete', freshness_status: 'fixture', last_complete_date: '2026-09-18', warnings: ['时效未验证'] },
        bars: older
          ? [
              { time: '2026-09-11', open: '1400', high: '1410', low: '1390', close: '1405', volume: '800', is_final: true },
              { time: '2026-09-12', open: '1405', high: '1420', low: '1400', close: '1418', volume: '900', is_final: true }
            ]
          : [
              { time: '2026-09-16', open: '1420', high: '1430', low: '1415', close: '1428', volume: '1000', is_final: true },
              { time: '2026-09-17', open: '1428', high: '1440', low: '1420', close: '1435', volume: '1100', is_final: true },
              { time: '2026-09-18', open: '1435', high: '1450', low: '1430', close: '1448', volume: '1200', is_final: true }
            ],
        has_more: !older,
        next_cursor: older ? null : 'older1'
      }))
    }
    if (url.includes('/indicator-series')) {
      return fulfill(wrap({ series: [], times: ['2026-09-16', '2026-09-17', '2026-09-18'] }))
    }
    return fulfill(wrap({ items: [], revision: 0 }))
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/strategies')
  const olderReq = page.waitForRequest((req) => req.url().includes('/market/ohlcv') && req.url().includes('cursor='), { timeout: 8000 })
  await page.getByLabel('搜索股票').fill('600519')
  await expect(page.getByRole('option').first()).toBeVisible()
  await page.getByRole('option').first().click()
  await expect(page.getByTestId('quote-date')).toHaveText('2026-09-18')
  await expect(page.getByTestId('chart-host')).toBeVisible()
  const autoOlder = await olderReq.catch(() => null)
  if (!autoOlder) {
    await page.getByRole('button', { name: '更早行情' }).click()
  }
  await page.getByTestId('chart-reset').click()
  const box = await page.getByTestId('chart-host').boundingBox()
  expect(box?.height || 0).toBeGreaterThan(200)
  await page.mouse.move(box.x + box.width * 0.75, box.y + 80)
  await expect(page.getByTestId('quote-strip')).toContainText('开')
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
