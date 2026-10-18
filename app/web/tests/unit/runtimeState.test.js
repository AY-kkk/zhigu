import { describe, expect, it } from 'vitest'
import { normalizeRuntimeState } from '../../src/utils/runtimeState'

describe('normalizeRuntimeState', () => {
  it('keeps four dimensions independent', () => {
    expect(normalizeRuntimeState({
      deployment_environment: 'demo',
      operation_mode: 'read_only',
      data_provenance: 'fixture',
      health: 'degraded',
      reason: 'MODEL_UNAVAILABLE',
    })).toEqual({
      deploymentEnvironment: 'demo',
      operationMode: 'read_only',
      dataProvenance: 'fixture',
      health: 'degraded',
      reason: 'MODEL_UNAVAILABLE',
      blocking: true,
    })
  })

  it('fail-closes production fixture and illegal values', () => {
    const value = normalizeRuntimeState({
      deployment_environment: 'production',
      operation_mode: 'live',
      data_provenance: 'fixture',
      health: 'ready',
    })
    expect(value.dataProvenance).toBe('real')
    expect(value.operationMode).toBe('read_only')
    expect(value.health).toBe('unavailable')
    expect(value.reason).toBe('INVALID_DATA_PROVENANCE')
  })
})
