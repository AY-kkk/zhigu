import { intelRequest } from '../utils/intelHttp.js'

export const getSession = (mode, config = {}) => intelRequest('/session', { mode, ...config })
export const createDemoSession = (idempotencyKey, config = {}) =>
  intelRequest('/demo-sessions', { method: 'POST', mode: 'demo', body: { fixture_set: 'events-v1' }, idempotencyKey, ...config })
export const searchInstruments = (mode, q, limit = 20, config = {}) =>
  intelRequest(`/instruments?q=${encodeURIComponent(q)}&limit=${limit}`, { mode, ...config })
export const getWatchlist = (mode, config = {}) => intelRequest('/watchlist', { mode, ...config })
export const addWatchlist = (mode, code, config = {}) =>
  intelRequest(`/watchlist/${encodeURIComponent(code)}`, { method: 'PUT', mode, body: {}, ...config })
export const removeWatchlist = (mode, code, config = {}) =>
  intelRequest(`/watchlist/${encodeURIComponent(code)}`, { method: 'DELETE', mode, ...config })
export const listEvents = (mode, params = {}, config = {}) => {
  const query = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => { if (value !== '' && value != null) query.set(key, value) })
  const suffix = query.toString() ? `?${query}` : ''
  return intelRequest(`/events${suffix}`, { mode, ...config })
}
export const getEvent = (mode, id, version, config = {}) =>
  intelRequest(`/events/${encodeURIComponent(id)}${version ? `?version=${version}` : ''}`, { mode, ...config })
export const getTimeline = (mode, id, config = {}) =>
  intelRequest(`/events/${encodeURIComponent(id)}/timeline`, { mode, ...config })
export const getEvidence = (mode, id, params = {}, config = {}) => {
  const query = new URLSearchParams(params)
  return intelRequest(`/events/${encodeURIComponent(id)}/evidence?${query}`, { mode, ...config })
}
export const getConflicts = (mode, id, config = {}) =>
  intelRequest(`/events/${encodeURIComponent(id)}/conflicts`, { mode, ...config })
export const getChanges = (mode, id, config = {}) =>
  intelRequest(`/events/${encodeURIComponent(id)}/changes`, { mode, ...config })
export const getSourceRevision = (mode, id, config = {}) =>
  intelRequest(`/source-revisions/${encodeURIComponent(id)}`, { mode, ...config })
export const listNotifications = (mode, params = {}, config = {}) => {
  const query = new URLSearchParams(params)
  return intelRequest(`/notifications?${query}`, { mode, ...config })
}
export const patchNotification = (mode, id, status, config = {}) =>
  intelRequest(`/notifications/${encodeURIComponent(id)}`, { method: 'PATCH', mode, body: { status }, ...config })
export const setMute = (mode, id, muted, config = {}) =>
  intelRequest(`/events/${encodeURIComponent(id)}/mute`, { method: 'PUT', mode, body: { muted }, ...config })
export const getDataStatus = (mode, config = {}) => intelRequest('/data-status', { mode, ...config })
export const replay = (mode, body, idempotencyKey, config = {}) =>
  intelRequest('/replay/actions', { method: 'POST', mode, body, idempotencyKey, ...config })
export const listReviewItems = (mode, params = {}, config = {}) => {
  const query = new URLSearchParams(params)
  return intelRequest(`/admin/review-items?${query}`, { mode, ...config })
}
export const resolveReviewItem = (mode, id, body, idempotencyKey, config = {}) =>
  intelRequest(`/admin/review-items/${encodeURIComponent(id)}/resolve`, { method: 'POST', mode, body, idempotencyKey, ...config })
export const createIngestionJob = (mode, body, idempotencyKey, config = {}) =>
  intelRequest('/admin/ingestion-jobs', { method: 'POST', mode, body, idempotencyKey, ...config })
export const getIngestionJob = (mode, id, config = {}) =>
  intelRequest(`/admin/ingestion-jobs/${encodeURIComponent(id)}`, { mode, ...config })
export const importSourceRevision = (mode, body, idempotencyKey, config = {}) =>
  intelRequest('/admin/source-revisions', { method: 'POST', mode, body, idempotencyKey, ...config })
export const reassignEvidence = (mode, id, body, idempotencyKey, config = {}) =>
  intelRequest(`/admin/events/${encodeURIComponent(id)}/reassign`, { method: 'POST', mode, body, idempotencyKey, ...config })
