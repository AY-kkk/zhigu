const ENVIRONMENTS = new Set(['test', 'demo', 'production'])
const MODES = new Set(['off', 'read_only', 'live'])
const PROVENANCE = new Set(['fixture', 'real'])
const HEALTH = new Set(['ready', 'degraded', 'unavailable'])

export function normalizeRuntimeState(input = {}) {
  const value = {
    deploymentEnvironment: ENVIRONMENTS.has(input.deployment_environment) ? input.deployment_environment : 'test',
    operationMode: MODES.has(input.operation_mode) ? input.operation_mode : 'off',
    dataProvenance: PROVENANCE.has(input.data_provenance) ? input.data_provenance : 'fixture',
    health: HEALTH.has(input.health) ? input.health : 'unavailable',
    reason: String(input.reason || ''),
    blocking: true,
  }
  if (value.deploymentEnvironment === 'production' && value.dataProvenance === 'fixture') {
    value.dataProvenance = 'real'
    value.operationMode = 'read_only'
    value.health = 'unavailable'
    value.reason = 'INVALID_DATA_PROVENANCE'
  }
  value.blocking = value.operationMode !== 'live' || value.health !== 'ready'
  return value
}
