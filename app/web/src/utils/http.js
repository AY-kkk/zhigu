import axios from 'axios'

let csrfToken = ''
let bearerToken = ''

const http = axios.create({
  baseURL: '/',
  timeout: 20000,
  withCredentials: true,
})

export function setBearerToken(token) {
  bearerToken = typeof token === 'string' ? token : ''
}

export function setCsrfToken(token) {
  csrfToken = typeof token === 'string' ? token : ''
}

export function clearAuthCredential() {
  bearerToken = ''
  csrfToken = ''
}

export function getCsrfToken() {
  return csrfToken
}

export function hasAuthCredential() {
  return Boolean(bearerToken || csrfToken)
}

function transportError(error) {
  const body = error.response?.data
  if (body?.error) {
    const err = new Error(body.error.message || '请求失败')
    err.code = body.error.code
    err.status = error.response.status
    err.retryable = Boolean(body.error.retryable)
    err.traceId = body.trace_id || ''
    err.retryAfterSeconds = body.error.retry_after_seconds ?? null
    return err
  }
  const err = new Error(error.message || '请求失败')
  err.status = error.response?.status || 0
  err.traceId = ''
  err.retryable = false
  if (!error.response) {
    err.code = error.code === 'ECONNABORTED' ? 'TIMEOUT' : 'NETWORK'
  } else if (error.response.status === 401) {
    err.code = 'UNAUTHENTICATED'
  } else if (error.response.status >= 500) {
    err.code = 'INTERNAL'
    err.retryable = true
  }
  return err
}

http.interceptors.request.use((config) => {
  const method = String(config.method || 'get').toUpperCase()
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) {
    if (csrfToken) config.headers['X-CSRF-Token'] = csrfToken
    if (bearerToken && !config.headers.Authorization) config.headers.Authorization = `Bearer ${bearerToken}`
  }
  config.headers['X-Request-ID'] = crypto.randomUUID()
  return config
})

http.interceptors.response.use(
  (res) => {
    if (res.config?.responseType === 'blob') return res
    const body = res.data
    if (body && Object.prototype.hasOwnProperty.call(body, 'error')) {
      if (body.error) {
        const err = new Error(body.error.message || '请求失败')
        err.code = body.error.code
        err.status = res.status
        err.retryable = Boolean(body.error.retryable)
        err.traceId = body.trace_id || ''
        err.retryAfterSeconds = body.error.retry_after_seconds ?? null
        throw err
      }
      return { ...res, data: body.data, trace_id: body.trace_id }
    }
    return res
  },
  (error) => {
    const err = transportError(error)
    const url = String(error.config?.url || '')
    if (err.status === 401 && !url.includes('/auth/login')) {
      clearAuthCredential()
      window.dispatchEvent(new CustomEvent('zhigu:session-expired', { detail: { traceId: err.traceId } }))
    }
    throw err
  },
)

export default http
