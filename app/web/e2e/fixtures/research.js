export function wrap(data) {
  return { error: null, data, trace_id: 'e2e' }
}

export function errorBody(status, code, message) {
  return { error: { code, message }, data: null }
}

export const DEMO_CLAIM = '演示公司的收入增长能否支持未来一年股价上涨？这是一条超过二十个字的测试观点。'

export function parseSuccess({ candidates } = {}) {
  return wrap({
    draft_id: 'draft_e2e',
    revision: 1,
    candidates: candidates ?? [
      { instrument_id: 'DEMO:COMPANY', symbol: 'DEMO', name: '演示公司' }
    ],
    suggested_horizon: '未来一年',
    items: [
      { claim_id: 'c1', claim_type: 'fact', text: '收入增长被用作支持判断的事实。' },
      { claim_id: 'c2', claim_type: 'inference', text: '该事实被推断为支持未来一年股价。' }
    ],
    needs_confirmation: true,
    mode: 'fixture'
  })
}

export function historyPage(items, next = '') {
  return wrap({ items, next_cursor: next })
}

export function historyItem(overrides = {}) {
  return {
    run_id: 'run_e2e_1',
    status: 'completed',
    instrument_id: 'DEMO:COMPANY',
    mode: 'fixture',
    created_at: '2026-09-01T08:00:00Z',
    ...overrides
  }
}

export function evidenceDoc(id = 'ev_1') {
  return wrap({
    schema_version: '1.0',
    evidence_id: id,
    run_id: 'run_e2e_1',
    instrument_id: 'DEMO:COMPANY',
    source_id: 'src_1',
    title: '演示公司年度报告摘要',
    source_url: 'https://example.com/filing',
    source_kind: 'filing',
    published_at: '2026-03-01T00:00:00Z',
    available_at: '2026-03-02T00:00:00Z',
    retrieved_at: '2026-03-03T00:00:00Z',
    content_hash: 'a'.repeat(64),
    data_version: 'v1',
    mode: 'fixture',
    locator: '第12页',
    text: '营业收入同比增长。数值 0 必须保留。',
    metrics: [
      {
        metric: 'revenue',
        period_start: '2025-01-01',
        period_end: '2025-12-31',
        value: '0',
        unit: 'CNY',
        value_type: 'actual'
      }
    ]
  })
}

export function report(verdict, extras = {}) {
  const evidenceIds = extras.evidence_ids || ['ev_1', 'ev_2']
  const quality = extras.quality_status || (verdict ? 'completed' : 'incomplete')
  return {
    schema_version: '1.0',
    run_id: extras.run_id || 'run_e2e_1',
    version: 1,
    mode: 'fixture',
    as_of: '2026-09-01T08:00:00Z',
    quality_status: quality,
    verdict: quality === 'incomplete' ? null : verdict,
    summary: extras.summary || '这是一份用于界面验收的演示摘要，不构成投资建议。',
    support: extras.support ?? [
      { claim_type: 'fact', text: '已发布支持材料表明收入口径可核对。', evidence_ids: ['ev_1'] }
    ],
    challenge: extras.challenge ?? [
      { claim_type: 'inference', text: '已发布反证提示现金流与收入并非同步。', evidence_ids: ['ev_2'] }
    ],
    assumptions: extras.assumptions ?? ['假设行业需求保持稳定。'],
    change_conditions: extras.change_conditions ?? ['若现金流持续恶化，判断需要重估。'],
    unknowns: extras.unknowns ?? ['海外收入拆分尚未充分披露。'],
    evidence_ids: evidenceIds,
    model_config_version: 'm1',
    source_policy_version: 's1',
    prompt_version: 'p1'
  }
}

export function runView(overrides = {}) {
  const status = overrides.status || 'completed'
  const reportValue = Object.prototype.hasOwnProperty.call(overrides, 'report')
    ? overrides.report
    : report('supported', { run_id: overrides.run_id || 'run_e2e_1' })
  return wrap({
    run_id: overrides.run_id || 'run_e2e_1',
    status,
    stage: overrides.stage || (status === 'completed' ? 'done' : status),
    mode: 'fixture',
    as_of: '2026-09-01T08:00:00Z',
    instrument_id: 'DEMO:COMPANY',
    horizon: '未来一年',
    claim: { text: DEMO_CLAIM, horizon: '未来一年', items: [] },
    report: reportValue,
    warnings: [],
    error: overrides.error || null,
    updated_at: '2026-09-01T08:05:00Z',
    ...overrides,
    report: reportValue,
    status
  })
}
