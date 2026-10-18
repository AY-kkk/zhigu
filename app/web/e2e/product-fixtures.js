export const envelope = data => ({ data, error: null, trace_id: crypto.randomUUID() })

export async function installProductApi(page, overrides = {}) {
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url())
    const path = url.pathname
    // The glob also matches Vite source modules such as /src/api/session.js.
    // Let those requests reach the dev server and mock only real API routes.
    if (!path.startsWith('/api/')) return route.continue()
    for (const [suffix, value] of Object.entries(overrides)) {
      if (path.endsWith(suffix)) {
        return route.fulfill({ json: envelope(value) })
      }
    }
    if (path.endsWith('/session/csrf')) {
      return route.fulfill({ status: 200, headers: { 'X-CSRF-Token': 'e2e-csrf-token' }, json: envelope({ csrf_token: 'e2e-csrf-token' }) })
    }
    if (path.endsWith('/capabilities')) {
      return route.fulfill({ json: envelope({
        schema_version: 'futures.capabilities.v1',
        deployment_environment: 'test',
        operation_mode: 'read_only',
        data_provenance: 'fixture',
        health: 'degraded',
        reason: 'FIXTURE_TEST_ONLY',
        products: ['SHFE.CU'],
        features: { research: true, tracking: true, export: true },
        limits: { model_calls: 8, tool_calls: 24, tokens: 48000 },
      }) })
    }
    if (path.endsWith('/products')) {
      return route.fulfill({ json: envelope({ items: [{ id: 'SHFE.CU', product_id: 'SHFE.CU', name: '铜', exchange: 'SHFE', admission: 'admitted' }], limit: 20, cursor: null, next_cursor: null }) })
    }
    if (path.endsWith('/products/SHFE.CU/workbench')) {
      return route.fulfill({ json: envelope({ product_name: '铜', exchange: 'SHFE', product_kind: 'metal', coverage_ready: false }) })
    }
    if (path.endsWith('/notifications')) {
      return route.fulfill({ json: envelope({ items: [], limit: 20, cursor: null, next_cursor: null }) })
    }
    if (path.endsWith('/hypotheses')) {
      return route.fulfill({ json: envelope({ items: [], limit: 20, cursor: null, next_cursor: null }) })
    }
    if (path.endsWith('/runs')) {
      return route.fulfill({ json: envelope({ items: [], limit: 20, cursor: null, next_cursor: null }) })
    }
    return route.fulfill({ json: envelope({}) })
  })
}

export async function assertNoCredentialStorage(page) {
  const storage = await page.evaluate(() => ({
    local: Object.fromEntries(Object.entries(localStorage)),
    session: Object.fromEntries(Object.entries(sessionStorage)),
    url: location.href,
  }))
  const serialized = JSON.stringify(storage)
  for (const token of ['zhigu_token', 'zhigu_session', 'csrf_token', 'Bearer ', 'eyJ', 'BEGIN PRIVATE KEY']) {
    if (serialized.includes(token)) throw new Error(`credential-like value found in browser state: ${token}`)
  }
}
