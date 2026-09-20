import http from '../utils/http.js'

export function login(username, password) {
  return http.post('/api/finance/auth/login', { username, password })
}
export function parseClaim(text) {
  return http.post('/api/finance/claims/parse', { text })
}
export function createResearch(payload, idempotencyKey) {
  return http.post('/api/finance/research', payload, { headers: { 'Idempotency-Key': idempotencyKey } })
}
export function getResearch(id) {
  return http.get(`/api/finance/research/${id}`)
}
export function listResearch(params) {
  return http.get('/api/finance/research', { params })
}
export function cancelResearch(id) {
  return http.post(`/api/finance/research/${id}/cancel`, {})
}
export function deleteResearch(id) {
  return http.delete(`/api/finance/research/${id}`)
}
export function getEvidence(id) {
  return http.get(`/api/finance/evidence/${id}`)
}
export function askQuestion(id, text, key) {
  return http.post(`/api/finance/research/${id}/questions`, { text }, { headers: { 'Idempotency-Key': key } })
}
export function listInstruments() {
  return http.get('/api/finance/instruments')
}
