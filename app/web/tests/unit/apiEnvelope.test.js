import { describe, expect, it } from 'vitest'
import { normalizePage, unwrapEnvelope } from '../../src/utils/apiEnvelope'

describe('apiEnvelope', () => {
  it('preserves stable errors and trace IDs', () => {
    let caught
    try {
      unwrapEnvelope({
        data: null,
        error: { code: 'FUTURES_READ_ONLY', message: 'read only', retryable: false },
        trace_id: '550e8400-e29b-41d4-a716-446655440000',
      })
    } catch (error) {
      caught = error
    }
    expect(caught?.code).toBe('FUTURES_READ_ONLY')
    expect(caught?.message).toBe('read only')
    expect(caught?.traceId).toBe('550e8400-e29b-41d4-a716-446655440000')
    expect(caught?.retryable).toBe(false)
  })

  it('rejects malformed success envelopes and keeps pagination fields', () => {
    expect(() => unwrapEnvelope({ data: {}, error: {} })).toThrow()
    expect(normalizePage({ items: [], limit: 20, cursor: null, next_cursor: null })).toEqual({
      items: [], limit: 20, cursor: null, nextCursor: null,
    })
  })
})
