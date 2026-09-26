import axios from 'axios'

const http = axios.create({ baseURL: '/', timeout: 20000 })

function transportError(error) {
  const body = error.response?.data
  if (body?.error) {
    const err = new Error(body.error.message || '请求失败')
    err.code = body.error.code
    err.status = error.response.status
    return err
  }
  const err = new Error(error.message || '请求失败')
  err.status = error.response?.status || 0
  if (!error.response) {
    err.code = error.code === 'ECONNABORTED' ? 'TIMEOUT' : 'NETWORK'
  } else if (error.response.status === 401) {
    err.code = 'UNAUTHENTICATED'
  } else if (error.response.status >= 500) {
    err.code = 'INTERNAL'
  }
  return err
}

function clearSession() {
  localStorage.removeItem('zhigu_token')
  localStorage.removeItem('zhigu_role')
  localStorage.removeItem('zhigu_user')
}

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('zhigu_token')
  if (token) config.headers.Authorization = `Bearer ${token}`
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
      clearSession()
      if (!window.location.pathname.startsWith('/login')) {
        window.location.assign('/login')
      }
    }
    throw err
  }
)

export default http
