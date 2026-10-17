import http, { setCsrfToken } from '../utils/http.js'

export async function fetchCsrf() {
  const response = await http.get('/api/v1/session/csrf')
  const token = response.headers?.['x-csrf-token'] || response.data?.data?.csrf_token || response.data?.csrf_token
  if (token) setCsrfToken(token)
  return token || ''
}

export async function logout() {
  await http.post('/api/finance/auth/logout', {})
}
