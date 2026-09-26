import { expect, test } from '@playwright/test'
import { parseSuccess, wrap } from './fixtures/research.js'

const DEMO_CLAIM = '演示公司的收入增长能否支持未来一年股价上涨？这是一条超过二十个字的测试观点。'

async function seedSession(page) {
  await page.addInitScript(() => {
    localStorage.setItem('zhigu_token', 'e2e-token')
    localStorage.setItem('zhigu_user', 'invitee')
    localStorage.setItem('zhigu_role', 'invitee')
  })
}

async function mockResearchApi(page, mode) {
  await page.route('**/api/finance/**', async (route) => {
    const req = route.request()
    const url = new URL(req.url())
    const fulfill = (status, body) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
    if (req.method() === 'POST' && url.pathname.endsWith('/research-documents')) {
      return fulfill(201, wrap({
        document_id: 'doc_e2e',
        filename: 'sample-report.txt',
        media_type: 'text/plain',
        byte_size: 24,
        content_hash: 'hash',
        extraction_status: 'succeeded',
        page_count: 1,
        span_count: 1
      }))
    }
    if (url.pathname.endsWith('/instruments')) {
      return fulfill(200, wrap({ items: [{ instrument_id: 'DEMO:COMPANY', symbol: 'DEMO', name: '演示公司', market: 'A' }], mode: 'fixture' }))
    }
    if (req.method() === 'POST' && url.pathname.endsWith('/claims/parse')) {
      const body = JSON.parse(req.postData() || '{}')
      const data = parseSuccess({ candidates: [{ instrument_id: 'DEMO:COMPANY', symbol: 'DEMO', name: '演示公司' }] }).data
      data.input_mode = mode
      data.document_id = body.document_id || ''
      data.focus_text = body.focus_text || ''
      data.items = mode === 'report_only'
        ? [{ claim_id: 'claim_1', claim_type: 'inference', text: '研报认为公司收入将继续增长。', source_span_id: 'span_1' }]
        : data.items
      return fulfill(200, wrap(data))
    }
    return fulfill(404, wrap(null))
  })
}

test('claim, report, and combined inputs reach confirmation', async ({ page }) => {
  for (const mode of ['claim_only', 'report_only', 'claim_and_report']) {
    await seedSession(page)
    await mockResearchApi(page, mode)
    await page.goto('/app/research/new')
    if (mode !== 'report_only') await page.getByTestId('test_base_textarea').fill(DEMO_CLAIM)
    if (mode !== 'claim_only') {
      await page.locator('#research-document-input').setInputFiles({
        name: 'sample-report.txt',
        mimeType: 'text/plain',
        buffer: Buffer.from('研报认为公司收入将继续增长。')
      })
    }
    await page.getByRole('button', { name: '解析观点' }).click()
    const label = { claim_only: '仅观点', report_only: '仅研报', claim_and_report: '观点 + 研报' }[mode]
    await expect(page.getByTestId('input-mode')).toContainText(label)
    await expect(page.getByRole('button', { name: '确认并开始研究' })).toBeVisible()
  }
})
