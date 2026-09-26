import { test, expect } from '@playwright/test'
import assert from 'node:assert/strict'

// R7：真实 Vue → Go → PostgreSQL/worker 链路（复制→编辑→保存→回测→结果）。
// 实际请求全部走真实后端，不替换为静态成功体；行情为服务端明确标识的 fixture
// （ZHIGU_MARKET_MODE=fixture），版本一致性显式断言。

test.describe('strategy market real chain', () => {
  test.setTimeout(150000)

  test('copy edit save backtest use same version', async ({ page, request }) => {
    const login = async (username) => {
      const res = await request.post('/api/finance/auth/login', { data: { username, password: 'Passw0rd!' } })
      expect(res.ok()).toBeTruthy()
      return (await res.json()).data
    }
    const admin = await login('admin')
    const user = await login('invitee')
    expect(admin.role).toBe('admin')

    // 真实目录与真实 K 线（fixture 明确标识）。
    const instRes = await request.get('/api/finance/market/instruments?limit=10', {
      headers: { Authorization: `Bearer ${user.token}` }
    })
    const insts = (await instRes.json()).data.items
    const stock = insts.find((i) => i.asset_type === 'stock')
    expect(stock).toBeTruthy()
    const ohlcRes = await request.get(
      `/api/finance/market/ohlcv?instrument_id=${encodeURIComponent(stock.instrument_id)}&period=1d&adjust=raw&limit=400`,
      { headers: { Authorization: `Bearer ${user.token}` } }
    )
    const bars = (await ohlcRes.json()).data.bars
    expect(bars.length).toBeGreaterThan(30)
    const start = bars[Math.max(0, bars.length - 250)].time
    const end = bars[bars.length - 1].time

    // 管理员真实上架：创建 → 校验 → 发布。
    const key = `chain-${Date.now()}`
    const version = {
      name: '链路连续放量策略',
      summary: '真实链路验证用策略，历史模拟结果不代表未来收益',
      category: '趋势',
      tags: ['链路验证'],
      markets: [stock.instrument_id.endsWith('.HK') ? 'HK' : 'A'],
      signal_period: '1d',
      description: '连续放量时买入。',
      hypothesis: '放量延续。',
      failure_cases: '缩量震荡失效。',
      sources: [{ title: '平台整理', record_ref: 'CHAIN-1', collected_at: '2026-09-20', adaptation: '平台整理' }],
      rights_note: '仅限链路验证展示',
      editor_schema_version: 'strategy.editor.v1',
      rule_template: {
        schema_version: 'strategy.market.v1',
        editor_state: {
          name: '链路连续放量策略',
          instrument_id: null,
          signal_period: '1d',
          price_basis: 'raw',
          indicators: [{ id: 'ma', type: 'MA', params: { n: 2 } }],
          entry: {
            all: [{ kind: 'volume_increase', days: 3 }, { op: 'gt', left: 'close', right: { constant: '0' } }]
          },
          exit: { op: 'lt', left: 'close', right: { constant: '0' } },
          position: { type: 'equity_fraction', value: '1' },
          risk: { check: 'close' },
          execution: { timing: 'next_session_open', priority: 'exit_first' }
        },
        instrument_binding: { mode: 'select_one', markets: ['A', 'HK'] }
      },
      backtest_defaults: {}
    }
    const created = await request.post('/api/admin/strategy-market/items', {
      headers: { Authorization: `Bearer ${admin.token}`, 'Idempotency-Key': key },
      data: { slug: key, version }
    })
    expect(created.status()).toBe(201)
    const item = (await created.json()).data
    const validated = await request.post(
      `/api/admin/strategy-market/items/${item.item_id}/versions/${item.version_id}/validate`,
      {
        headers: { Authorization: `Bearer ${admin.token}`, 'Idempotency-Key': `${key}-v` },
        data: { revision: 1, validation_instrument_id: stock.instrument_id }
      }
    )
    expect((await validated.json()).data.validation_status).toBe('passed')
    const published = await request.post(`/api/admin/strategy-market/items/${item.item_id}/publish`, {
      headers: { Authorization: `Bearer ${admin.token}`, 'Idempotency-Key': `${key}-p` },
      data: { revision: 2, market_version_id: item.version_id, reason: '链路验证' }
    })
    expect(published.status()).toBe(201)

    // UI：市场详情复制（无标的 → 待补全草稿），再在工作台真实补全（R6 缺项流）。
    await page.addInitScript((tok) => {
      localStorage.setItem('zhigu_token', tok)
      localStorage.setItem('zhigu_user', 'invitee')
      localStorage.setItem('zhigu_role', 'user')
    }, user.token)
    await page.goto(`/app/strategies/market/${item.item_id}`)
    // 市场详情 → 复制（真实 POST /copies）。
    const copyPromise = page.waitForResponse((r) => r.url().includes('/copies') && r.request().method() === 'POST')
    await page.getByRole('button', { name: '复制到我的策略' }).click()
    await expect(page).toHaveURL(/draft_id=/)
    const copyData = (await (await copyPromise).json()).data

    // 工作台：选真实标的 → 规则窗口补全标的与仓位 → 应用（真实 PATCH，CAS revision 前进）。
    await page.getByPlaceholder(/代码、名称、拼音/).fill(stock.code || stock.instrument_id)
    await page.getByRole('option').first().click()
    await page.getByRole('button', { name: '配置买卖规则' }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByTestId('use-workspace-instrument').click()
    await dialog.getByTestId('position-value').fill('0.5')
    const patchPromise = page.waitForResponse(
      (r) => /strategy-drafts\/[^/]+$/.test(new URL(r.url()).pathname) && r.request().method() === 'PATCH'
    )
    await dialog.getByTestId('apply-rules').click()
    const patched = (await (await patchPromise).json()).data
    expect(patched.revision).toBeGreaterThan(copyData.revision)
    expect(patched.status).toBe('ready')

    // 回测区间来自真实 K 线。
    await page.getByTestId('backtest-start').fill(start)
    await page.getByTestId('backtest-end').fill(end)

    // 保存与回测：显式断言回测使用保存的同一版本（R7-E1）。
    const savePromise = page.waitForResponse(
      (r) => /\/api\/finance\/strategies$/.test(new URL(r.url()).pathname) && r.request().method() === 'POST'
    )
    const btPromise = page.waitForResponse(
      (r) => /\/api\/finance\/backtests$/.test(new URL(r.url()).pathname) && r.request().method() === 'POST'
    )
    await page.getByTestId('run-backtest').click()
    const saved = (await (await savePromise).json()).data
    const btReq = JSON.parse((await btPromise).request().postData() || '{}')
    assert.equal(btReq.strategy_version_id, saved.version_id, 'backtest must run the saved version')
    await expect(page.getByTestId('backtest-results')).toBeVisible()
    await expect(page.getByText('总收益')).toBeVisible()
  })
})
