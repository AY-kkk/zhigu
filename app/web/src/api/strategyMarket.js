import http from '../utils/http.js'

// §12.5 端点封装：组件不拼 URL、不手动解多层信封（utils/http.js 已解一层）。
export function listMarketItems(params, config = {}) {
  return http.get('/api/finance/strategy-market/items', { ...config, params })
}

export function getMarketItem(id, config = {}) {
  return http.get(`/api/finance/strategy-market/items/${encodeURIComponent(id)}`, config)
}

export function getMarketEvidence(id, evidenceId, params, config = {}) {
  return http.get(
    `/api/finance/strategy-market/items/${encodeURIComponent(id)}/evidence/${encodeURIComponent(evidenceId)}`,
    { ...config, params }
  )
}

export function copyMarketItem(id, payload, key, config = {}) {
  return http.post(`/api/finance/strategy-market/items/${encodeURIComponent(id)}/copies`, payload, {
    ...config,
    headers: { ...(config.headers || {}), 'Idempotency-Key': key }
  })
}
