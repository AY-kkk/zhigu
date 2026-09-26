import { expect, test } from '@playwright/test'
import { wrap } from './fixtures/research.js'

const reportV2 = {
  schema_version: 'research-report.v2',
  run_id: 'run_viewpoint_1',
  version: 1,
  mode: 'fixture',
  as_of: '2026-09-01T08:00:00Z',
  quality_status: 'completed',
  verdict: 'partially_supported',
  summary: '核心事实有一定支持，但推理链仍需独立证据。',
  support: [],
  challenge: [],
  assumptions: ['围绕现金流改善的假设尚未独立证明。'],
  change_conditions: ['若证伪条件触发，重新评估该主张。'],
  unknowns: ['无法取数：缺少独立现金流证据。'],
  fact_checks: [{ claim_id: 'c1', status: 'uncertain', reason: '支持与挑战证据均存在', evidence_ids: ['ev_1'] }],
  challenges: [{ claim_id: 'c1', title: '现金流反证', argument: '现金流下降会削弱盈利质量改善。', evidence_ids: ['ev_2'] }],
  reasoning_gaps: [{ claim_id: 'c1', from: '收入增长', to: '盈利质量改善', missing: '缺少利润与现金流同步的独立证据。' }],
  tail_risks: [{ claim_id: 'c1', title: '现金流风险', description: '需要跟踪现金流是否持续恶化。', evidence_ids: [] }],
  test_conditions: [{ claim_id: 'c1', metric: '经营现金流', baseline: '本次证据', confirm_direction: '现金流持续改善', falsify_direction: '现金流持续恶化', review_at: '2026-12-31', trigger: '新公告' }],
  evidence_ids: ['ev_1', 'ev_2'],
  evidence_index: [
    { evidence_id: 'ev_1', source_grade: 'structured_data', verification_status: 'independent_verified', relation: 'support', title: '结构化数据' },
    { evidence_id: 'ev_2', source_grade: 'user_report', verification_status: 'reported_only', relation: 'challenge', title: '上传研报', locator: 'page 1' }
  ]
}

const runView = wrap({
  run_id: 'run_viewpoint_1',
  status: 'completed',
  stage: 'done',
  mode: 'fixture',
  as_of: '2026-09-01T08:00:00Z',
  instrument_id: 'DEMO:COMPANY',
  horizon: '2026-01-01/2026-12-31',
  claim: { text: '收入增长会证明盈利质量改善', horizon: '2026-01-01/2026-12-31', items: [{ claim_id: 'c1', text: '收入增长会证明盈利质量改善', claim_type: 'inference' }] },
  claim_results: [],
  report: reportV2
})

test('viewpoint report renders all required sections and source grade', async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('zhigu_token', 'e2e-token')
    localStorage.setItem('zhigu_user', 'invitee')
    localStorage.setItem('zhigu_role', 'invitee')
  })
  await page.route('**/api/finance/**', async (route) => {
    const url = new URL(route.request().url())
    const fulfill = (body) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) })
    if (url.pathname.endsWith('/api/finance/research/run_viewpoint_1')) return fulfill(runView)
    return fulfill(wrap({}))
  })
  await page.goto('/app/research/run_viewpoint_1')
  for (const heading of ['核心判断', '事实核验', '逐条质疑', '推理链缺口', '被忽略的风险', '证实与证伪条件', '证据清单']) {
    await expect(page.getByRole('heading', { name: heading })).toBeVisible()
  }
  await expect(page.getByText('研报陈述，未独立核验')).toBeVisible()
  await expect(page.getByRole('link', { name: '下载 HTML 报告' })).toBeVisible()
})
