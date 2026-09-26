import { test, expect } from '@playwright/test'

const wrap = (data, meta = {}) => ({ data, error: null, trace_id: 'e2e-intel', meta: { request_id: 'e2e-intel', as_of: '2026-09-26T00:00:00Z', coverage: { status: 'complete', scope: 'events-v1', last_success_at: '2026-09-26T00:00:00Z', pending_count: 0, gaps: [] }, warnings: ['演示数据'], ...meta } })

function mockIntel(page) {
  const state = { watched: [], step: 0, notifications: [] }
  page.route('**/api/finance/intel/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace('/api/finance/intel/v1', '')
    const method = request.method()
    const fulfill = (data, status = 200, headers = {}) => route.fulfill({ status, contentType: 'application/json', headers, body: JSON.stringify(wrap(data)) })
    const fail = (status, code, message, headers = {}) => route.fulfill({ status, contentType: 'application/json', headers, body: JSON.stringify({ data: null, error: { code, message }, trace_id: 'e2e-intel', meta: null }) })
    if (path === '/session' && method === 'GET') {
      if (!request.headers().cookie?.includes('zhigu_intel_demo=')) {
        return fail(401, 'DEMO_SESSION_EXPIRED', '演示会话不存在', { 'set-cookie': 'zhigu_intel_bootstrap=boot-e2e; Path=/api/finance/intel/v1; HttpOnly; SameSite=Lax' })
      }
      return fulfill({ mode: 'demo', principal_id: 'demo-e2e', role: 'user', namespace_label: '演示数据', expires_at: '2026-09-27T00:00:00Z', replay_version: state.step, generation: 1, branch: 'main', step_index: state.step })
    }
    if (path === '/demo-sessions' && method === 'POST') {
      return route.fulfill({ status: 201, contentType: 'application/json', headers: { 'set-cookie': 'zhigu_intel_demo=session-e2e; Path=/api/finance/intel/v1; HttpOnly; SameSite=Lax' }, body: JSON.stringify(wrap({ session_id: 'session-e2e', expires_at: '2026-09-27T00:00:00Z', namespace_label: '演示数据', generation: 1, replay_version: 0 })) })
    }
    if (path === '/instruments') return fulfill({ items: [{ code: 'DEMO.A', name: '演示公司A', exchange: 'DEMO' }, { code: 'DEMO.B', name: '演示公司B', exchange: 'DEMO' }] })
    if (path === '/watchlist' && method === 'GET') return fulfill({ items: state.watched.map((code) => ({ code, name: code === 'DEMO.A' ? '演示公司A' : '演示公司B', subscribed_at: '2026-09-26T00:00:00Z' })) })
    if (path.startsWith('/watchlist/') && method === 'PUT') {
      const code = decodeURIComponent(path.split('/').pop())
      if (!state.watched.includes(code)) state.watched.push(code)
      return fulfill({ code, subscribed_at: '2026-09-26T00:00:00Z' })
    }
    if (path.startsWith('/watchlist/') && method === 'DELETE') {
      const code = decodeURIComponent(path.split('/').pop())
      state.watched = state.watched.filter((x) => x !== code)
      return route.fulfill({ status: 204, body: '' })
    }
    if (path === '/replay/actions' && method === 'POST') {
      state.step += 1
      if (state.step === 1) state.notifications.push({ id: 'ntf-1', event_id: 'evt_demo_acq', title: '事件情报更新', reason: '固定回放步骤 S1', created_at: '2026-09-26T00:00:00Z', status: 'unread' })
      return fulfill({ replay_version: state.step, generation: 1, step_index: state.step, simulated_at: '2026-09-26T00:00:00Z', event_version: state.step, notification_count: state.notifications.length, change_ids: state.step === 1 ? ['chg_S1'] : [], done: false })
    }
    if (path === '/events' && method === 'GET') {
      return fulfill({ items: state.step ? [{ event_id: 'evt_demo_acq', type: 'acquisition', title: 'DEMO.A 收购 DEMO.B', subjects: [{ code: 'DEMO.A', role: 'subject', evidence_ids: [] }, { code: 'DEMO.B', role: 'counterparty', evidence_ids: [] }], verification: state.step === 1 ? 'unverified' : 'confirmed', phase: state.step === 1 ? 'unknown' : 'proposed', freshness: 'fresh', support_level: state.step === 1 ? 'low' : 'high', event_version: state.step, updated_at: '2026-09-26T00:00:00Z', open_conflict_count: 0 }] : [], next_cursor: '' })
    }
    if (path === '/events/evt_demo_acq' && method === 'GET') return fulfill({ event_id: 'evt_demo_acq', title: 'DEMO.A 收购 DEMO.B', core_claim: 'DEMO.A 存在收购 DEMO.B 的该项交易安排', verification: 'confirmed', phase: 'proposed', freshness: 'fresh', support_level: 'high', selected_version: state.step, latest_version: state.step, current_values: [{ field: 'transaction_amount', value: '800000000', currency: 'CNY' }], subjects: [{ code: 'DEMO.A', role: 'subject', evidence_ids: [] }] })
    if (path === '/events/evt_demo_acq/timeline') return fulfill({ items: [{ node_id: 'node-1', kind: 'source', title: 'DEMO.A 公告', excerpt: '公司正在推进收购B。', disclosed_at: '2026-09-10T01:00:00Z', recorded_at: '2026-09-10T01:00:00Z' }] })
    if (path === '/events/evt_demo_acq/evidence') return fulfill({ items: [{ evidence_id: 'ev_n1_core', claim_key: '__core__', quote: { text: '公司正在推进收购B' }, grade: 'fact', stance: 'support', weight: '1', status: 'active', source_revision_id: 'rev_n1' }] })
    if (path === '/events/evt_demo_acq/conflicts') return fulfill({ items: [] })
    if (path === '/events/evt_demo_acq/changes') return fulfill({ items: [{ change_id: 'chg_S1', event_version: state.step || 1, kind: 'new_event', summary: '事件情报更新：S1', recorded_at: '2026-09-26T00:00:00Z' }] })
    if (path === '/notifications') return fulfill({ items: state.notifications, next_cursor: '', unread_count: state.notifications.filter((x) => x.status === 'unread').length })
    if (path.startsWith('/notifications/') && method === 'PATCH') {
      const id = path.split('/').pop()
      const item = state.notifications.find((x) => x.id === id)
      if (item) item.status = JSON.parse(request.postData() || '{}').status
      return fulfill(item || {})
    }
    if (path === '/data-status') return fulfill({ providers: [{ provider: 'fixture', status: 'enabled', rights: 'demo-summary-only' }], coverage: { status: 'complete', scope: 'events-v1', last_success_at: '2026-09-26T00:00:00Z', pending_count: 0, gaps: [] } })
    return fail(404, 'NOT_FOUND', '未找到')
  })
  return state
}

test.use({ video: { mode: 'on', size: { width: 1280, height: 720 } } })
test.setTimeout(90000)

test('record the 60 second event intelligence demo', async ({ page }) => {
  mockIntel(page)
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto('/app/intel/demo')
  await expect(page.getByRole('button', { name: '开始演示' })).toBeVisible()
  await page.getByRole('button', { name: '开始演示' }).click()
  await expect(page.getByRole('heading', { name: '谁最先说，事实怎么变，哪些说法冲突，现在是什么状态？' })).toBeVisible()
  await page.waitForTimeout(6000)
  await page.getByRole('button', { name: '关注设置' }).click()
  await expect(page.getByRole('heading', { name: '关注设置' })).toBeVisible()
  await page.waitForTimeout(4000)
  await page.getByRole('button', { name: '添加' }).first().click()
  await expect(page.getByText('DEMO.A', { exact: true }).first()).toBeVisible()
  await page.waitForTimeout(5000)
  await page.getByRole('button', { name: '事件流', exact: true }).click()
  await page.getByRole('button', { name: '下一步' }).click()
  await expect(page.getByText('DEMO.A 收购 DEMO.B')).toBeVisible()
  await page.waitForTimeout(6000)
  await page.getByText('DEMO.A 收购 DEMO.B').click()
  await expect(page.getByText('DEMO.A 存在收购 DEMO.B 的该项交易安排')).toBeVisible()
  await page.waitForTimeout(7000)
  await page.mouse.wheel(0, 700)
  await expect(page.getByText('证据与出处')).toBeVisible()
  await page.waitForTimeout(6000)
  await page.mouse.wheel(0, 700)
  await expect(page.getByRole('heading', { name: '时间线', exact: true })).toBeVisible()
  await page.waitForTimeout(6000)
  await page.getByRole('button', { name: '通知中心' }).click()
  await expect(page.getByText('固定回放步骤 S1')).toBeVisible()
  await page.waitForTimeout(6000)
  await page.getByRole('button', { name: '回放' }).click()
  await expect(page.getByRole('heading', { name: '演示数据时间线' })).toBeVisible()
  await page.waitForTimeout(6000)
  await page.getByRole('button', { name: '事件流', exact: true }).click()
  await expect(page.getByText('演示数据').first()).toBeVisible()
  await page.waitForTimeout(12000)
})
