import http from '../utils/http.js'

export function searchInstruments(params, config = {}) {
  return http.get('/api/finance/market/instruments', { ...config, params })
}
export function getInstrument(id, config = {}) {
  return http.get(`/api/finance/market/instruments/${encodeURIComponent(id)}`, config)
}
export function getOHLCV(params, config = {}) {
  return http.get('/api/finance/market/ohlcv', { ...config, params })
}
export function listIndicators(config = {}) {
  return http.get('/api/finance/market/indicators', config)
}
export function indicatorSeries(payload, config = {}) {
  return http.post('/api/finance/market/indicator-series', payload, config)
}
export function getWorkspace(config = {}) {
  return http.get('/api/finance/strategies/workspace', config)
}
export function putWorkspace(payload, config = {}) {
  return http.put('/api/finance/strategies/workspace', payload, config)
}
export function generateDraft(payload, key, config = {}) {
  return http.post('/api/finance/strategy-drafts/generate', payload, {
    ...config,
    headers: { ...(config.headers || {}), 'Idempotency-Key': key }
  })
}
export function getGeneration(id, config = {}) {
  return http.get(`/api/finance/strategy-generations/${id}`, config)
}
export function cancelGeneration(id, config = {}) {
  return http.post(`/api/finance/strategy-generations/${id}/cancel`, {}, config)
}
export function getDraft(id, config = {}) {
  return http.get(`/api/finance/strategy-drafts/${id}`, config)
}
export function patchDraft(id, payload, config = {}) {
  return http.patch(`/api/finance/strategy-drafts/${id}`, payload, config)
}
export function saveStrategy(payload, key, config = {}) {
  return http.post('/api/finance/strategies', payload, {
    ...config,
    headers: { ...(config.headers || {}), 'Idempotency-Key': key }
  })
}
export function saveStrategyVersion(id, payload, key, config = {}) {
  return http.post(`/api/finance/strategies/${id}/versions`, payload, {
    ...config,
    headers: { ...(config.headers || {}), 'Idempotency-Key': key }
  })
}
export function listStrategies(params, config = {}) {
  return http.get('/api/finance/strategies', { ...config, params })
}
export function getStrategy(id, params, config = {}) {
  return http.get(`/api/finance/strategies/${id}`, { ...config, params })
}
export function deleteStrategy(id, config = {}) {
  return http.delete(`/api/finance/strategies/${id}`, config)
}
export function createBacktest(payload, key, config = {}) {
  return http.post('/api/finance/backtests', payload, {
    ...config,
    headers: { ...(config.headers || {}), 'Idempotency-Key': key }
  })
}
export function getBacktest(id, config = {}) {
  return http.get(`/api/finance/backtests/${id}`, config)
}
export function getBacktestResults(id, config = {}) {
  return http.get(`/api/finance/backtests/${id}/results`, config)
}
export function getBacktestTrades(id, config = {}) {
  return http.get(`/api/finance/backtests/${id}/trades`, config)
}
export function cancelBacktest(id, config = {}) {
  return http.post(`/api/finance/backtests/${id}/cancel`, {}, config)
}
export function exportBacktest(id, config = {}) {
  return http.get(`/api/finance/backtests/${id}/export`, { ...config, responseType: 'blob' })
}
export function listBacktests(params, config = {}) {
  return http.get('/api/finance/backtests', { ...config, params })
}
export function importDraft(payload, config = {}) {
  return http.post('/api/finance/strategy-drafts', payload, config)
}
