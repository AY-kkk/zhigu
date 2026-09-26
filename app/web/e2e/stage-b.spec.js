import { test, expect } from '@playwright/test'
import {
  DEMO_CLAIM,
  errorBody,
  parseSuccess,
  report,
  runView,
  wrap
} from './fixtures/research.js'

const HK_CANDIDATES = [
  { instrument_id: '00700.HK', symbol: '00700', name: '腾讯控股', market: 'HK' }
]

function claimInput(page) {
  return page.getByPlaceholder(/粘贴一个关于单家 A 股或港股公司的投资观点/)
}

async function seedSession(page) {
  await page.addInitScript(() => {
    localStorage.setItem('zhigu_token', 'e2e-token')
    localStorage.setItem('zhigu_user', 'invitee')
    localStorage.setItem('zhigu_role', 'invitee')
  })
}

async function mockApi(page, handlers = {}) {
  await page.route('**/api/finance/**', async (route) => {
    const req = route.request()
    const url = req.url()
    const method = req.method()
    const fulfill = (status, body) => route.fulfill({
      status,
      contentType: 'application/json',
      body: JSON.stringify(body)
    })
    if (url.includes('/api/finance/instruments')) {
      return fulfill(200, wrap({ items: HK_CANDIDATES, mode: 'live' }))
    }
    if (method === 'POST' && url.includes('/api/finance/claims/parse')) {
      if (handlers.parse) return handlers.parse(route, fulfill)
      return fulfill(200, parseSuccess({ candidates: HK_CANDIDATES }))
    }
    if (method === 'PATCH' && url.includes('/api/finance/claims/')) {
      if (handlers.patch) return handlers.patch(route, fulfill)
      const body = JSON.parse(req.postData() || '{}')
      if (Object.prototype.hasOwnProperty.call(body, 'as_of')) {
        return fulfill(400, errorBody(400, 'CLIENT_AS_OF', '客户端不得提交 as_of'))
      }
      return fulfill(200, parseSuccess({ candidates: HK_CANDIDATES }))
    }
    if (method === 'POST' && /\/api\/finance\/research$/.test(new URL(url).pathname)) {
      if (handlers.create) return handlers.create(route, fulfill)
      return fulfill(200, wrap({ run_id: 'run_e2e_1', status: 'queued', poll_url: '/api/finance/research/run_e2e_1' }))
    }
    if (method === 'GET' && url.includes('/api/finance/research/')) {
      if (handlers.get) return handlers.get(route, fulfill, url)
      return fulfill(200, runView({
        status: 'queued',
        report: null,
        instrument_id: '00700.HK'
      }))
    }
    if (method === 'GET' && /\/api\/finance\/research$/.test(new URL(url).pathname)) {
      return fulfill(200, wrap({ items: [], next_cursor: '' }))
    }
    return fulfill(404, errorBody(404, 'NOT_FOUND', 'not found'))
  })
}

test('b05 confirmation and real run', async ({ page }) => {
  let created = 0
  const createBodies = []
  await seedSession(page)
  await mockApi(page, {
    create: async (route, fulfill) => {
      created += 1
      const body = JSON.parse(route.request().postData() || '{}')
      createBodies.push(body)
      return fulfill(200, wrap({ run_id: 'run_e2e_1', status: 'queued', poll_url: '/api/finance/research/run_e2e_1' }))
    },
    get: async (_route, fulfill) => fulfill(200, runView({
      status: created ? 'queued' : 'researching',
      report: null,
      instrument_id: '00700.HK'
    }))
  })
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/app/research/new')
  await claimInput(page).fill(DEMO_CLAIM)
  await page.getByRole('button', { name: '解析观点' }).click()
  await expect(page.getByRole('button', { name: '确认并开始研究' })).toBeVisible()
  await expect(page.getByText(/腾讯控股/)).toBeVisible()
  await expect(page.getByText(/00700/)).toBeVisible()
  expect(created).toBe(0)
  await page.getByRole('button', { name: '确认并开始研究' }).click()
  await expect(page).toHaveURL(/\/app\/research\/run_e2e_1/, { timeout: 10000 })
  expect(created).toBe(1)
  expect(createBodies[0].as_of).toBeUndefined()
  expect(createBodies[0].draft_id).toBe('draft_e2e')
  expect(createBodies[0].revision).toBe(1)
})

test('b05 published report is visible only after complete', async ({ page }) => {
  await seedSession(page)
  await mockApi(page, {
    get: async (_route, fulfill) => fulfill(200, runView({
      status: 'completed',
      instrument_id: '00700.HK',
      report: report('supported', { summary: '腾讯控股年报科目可核对，不构成投资建议。' })
    }))
  })
  await page.goto('/app/research/run_e2e_1')
  await expect(page.getByText('腾讯控股年报科目可核对，不构成投资建议。')).toBeVisible()
})
