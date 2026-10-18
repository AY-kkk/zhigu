export class ContractError extends Error {
  constructor(code, message, options = {}) {
    super(message || code)
    this.name = 'ContractError'
    this.code = code
    this.retryable = Boolean(options.retryable)
    this.retryAfterSeconds = Number.isFinite(options.retryAfterSeconds) ? options.retryAfterSeconds : null
    this.traceId = options.traceId || ''
  }
}

export function unwrapEnvelope(value) {
  if (!value || typeof value !== 'object' || !('data' in value) || !('error' in value) || typeof value.trace_id !== 'string') {
    throw new ContractError('CONTRACT_ENVELOPE_INVALID', '响应不符合 envelope 契约')
  }
  if (value.error !== null) {
    const error = value.error
    if (!error || typeof error.code !== 'string') {
      throw new ContractError('CONTRACT_ENVELOPE_INVALID', '错误响应缺少稳定 code')
    }
    throw new ContractError(error.code, error.message, {
      retryable: error.retryable,
      retryAfterSeconds: error.retry_after_seconds,
      traceId: value.trace_id,
    })
  }
  return { data: value.data, traceId: value.trace_id }
}

export function normalizePage(value) {
  if (!value || !Array.isArray(value.items) || !Number.isInteger(value.limit)) {
    throw new ContractError('CONTRACT_PAGE_INVALID', '分页响应不符合契约')
  }
  return {
    items: value.items,
    limit: value.limit,
    cursor: value.cursor ?? null,
    nextCursor: value.next_cursor ?? null,
  }
}
