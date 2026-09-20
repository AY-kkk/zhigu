import axios from 'axios'

const http = axios.create({ baseURL: '/', timeout: 20000 })

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('zhigu_token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  config.headers['X-Request-ID'] = crypto.randomUUID()
  return config
})

http.interceptors.response.use(
  (res) => {
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
    const body = error.response?.data
    if (body?.error) {
      const err = new Error(body.error.message || '请求失败')
      err.code = body.error.code
      err.status = error.response.status
      throw err
    }
    throw error
  }
)

export default http
