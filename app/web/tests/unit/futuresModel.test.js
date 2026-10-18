import { describe, expect, it } from 'vitest'
import {
  canSubmitResearch,
  normalizeCapabilities,
  normalizeNotification,
  normalizeSourceProbe,
  notificationMatchesFilter,
  runDisplayState,
} from '../../src/modules/futures/model.js'

describe('futures model', () => {
  it('keeps capability dimensions and blocking reason', () => {
    const value = normalizeCapabilities({
      deployment_environment: 'production',
      operation_mode: 'read_only',
      data_provenance: 'real',
      health: 'unavailable',
      reason: 'MODEL_UNAVAILABLE',
      products: ['SHFE.CU'],
    })
    expect(value.operationMode).toBe('read_only')
    expect(value.dataProvenance).toBe('real')
    expect(value.health).toBe('unavailable')
    expect(value.blocking).toBe(true)
  })

  it('uses server run states and a local canceling display state', () => {
    expect(runDisplayState({ status: 'canceling' })).toMatchObject({ serverState: 'canceling', terminal: false })
    expect(runDisplayState({ status: 'succeeded' })).toMatchObject({ serverState: 'succeeded', terminal: true })
  })

  it('normalizes notification and source probe contracts', () => {
    const notification = normalizeNotification({ id: 'n1', type: 'run_failed', object_id: 'r1', object_version: 2, read_at: null, created_at: 'now' })
    expect(notification.severity).toBe('high')
    expect(notificationMatchesFilter(notification, { read: false })).toBe(true)
    const probe = normalizeSourceProbe({ id: 'p1', result: 'NOT_RUN', source_id: 's1' })
    expect(probe.result).toBe('NOT_RUN')
    expect(probe.releaseGateId).toBeNull()
  })

  it('blocks research without real readiness and accepted claims', () => {
    const result = canSubmitResearch({
      capabilities: { blocking: true, reason: 'MODEL_UNAVAILABLE' },
      input: { product_id: 'SHFE.CU', contract_id: 'CU-1' },
      acceptedClaims: [],
      parseState: 'failed',
    })
    expect(result.allowed).toBe(false)
    expect(result.blockingReasons).toContain('MODEL_UNAVAILABLE')
    expect(result.blockingReasons).toContain('CLAIMS_REQUIRED')
    expect(result.blockingReasons).toContain('PARSE_NOT_SUCCEEDED')
  })
})
