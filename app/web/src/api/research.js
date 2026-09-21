import http from '../utils/http.js'

export function login(username, password) {
  return http.post('/api/finance/auth/login', { username, password })
}
export function parseClaim(text, config = {}) {
  return http.post('/api/finance/claims/parse', { text }, config)
}
export function patchClaim(id, payload, config = {}) {
  return http.patch(`/api/finance/claims/${id}`, payload, config)
}
export function createResearch(payload, idempotencyKey, config = {}) {
  return http.post('/api/finance/research', payload, {
    ...config,
    headers: { ...(config.headers || {}), 'Idempotency-Key': idempotencyKey }
  })
}
export function getResearch(id, config = {}) {
  return http.get(`/api/finance/research/${id}`, config)
}
export function listResearch(params, config = {}) {
  return http.get('/api/finance/research', { ...config, params })
}
export function cancelResearch(id, config = {}) {
  return http.post(`/api/finance/research/${id}/cancel`, {}, config)
}
export function deleteResearch(id, config = {}) {
  return http.delete(`/api/finance/research/${id}`, config)
}
export function getEvidence(id, config = {}) {
  return http.get(`/api/finance/evidence/${id}`, config)
}
export function askQuestion(id, text, key, config = {}) {
  return http.post(`/api/finance/research/${id}/questions`, { text }, {
    ...config,
    headers: { ...(config.headers || {}), 'Idempotency-Key': key }
  })
}
export function listInstruments(params = {}, config = {}) {
  return http.get('/api/finance/instruments', { ...config, params })
}
