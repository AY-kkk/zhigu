const BASE = '/api/finance/intel/v1'

class IntelError extends Error {
  constructor(message, code, status, payload) {
    super(message)
    this.name = 'IntelError'
    this.code = code || 'NETWORK'
    this.status = status || 0
    this.payload = payload
  }
}

export async function intelRequest(path, { method = 'GET', mode = 'live', body, idempotencyKey, signal } = {}) {
  const headers = { Accept: 'application/json', 'X-Intel-Mode': mode, 'X-Request-ID': crypto.randomUUID() }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (idempotencyKey) headers['Idempotency-Key'] = idempotencyKey
  if (mode === 'live') {
    const token = localStorage.getItem('zhigu_token')
    if (token) headers.Authorization = `Bearer ${token}`
  }
  let response
  try {
    response = await fetch(`${BASE}${path}`, {
      method,
      headers,
      credentials: 'same-origin',
      body: body === undefined ? undefined : JSON.stringify(body),
      signal
    })
  } catch (error) {
    throw new IntelError(error?.message || '网络请求失败', error?.name === 'AbortError' ? 'ABORTED' : 'NETWORK')
  }
  let payload = null
  try { payload = await response.json() } catch (_) { /* 204 or malformed */ }
  if (!response.ok) {
    const error = payload?.error || {}
    const wrapped = new IntelError(error.message || `请求失败（${response.status}）`, error.code || `HTTP_${response.status}`, response.status, payload)
    if (mode === 'live' && response.status === 401) {
      localStorage.removeItem('zhigu_token')
      localStorage.removeItem('zhigu_role')
      localStorage.removeItem('zhigu_user')
      window.dispatchEvent(new CustomEvent('zhigu:intel-auth-expired', { detail: window.location.pathname + window.location.search }))
    }
    throw wrapped
  }
  return { data: payload?.data, meta: payload?.meta, trace_id: payload?.trace_id, status: response.status }
}

export function isDemoExpired(error) {
  return error?.code === 'DEMO_SESSION_EXPIRED' || (error?.status === 401 && error?.code !== 'UNAUTHENTICATED')
}
