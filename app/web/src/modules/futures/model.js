import { normalizeRuntimeState } from '../../utils/runtimeState.js'

const RUN_STATES = new Set(['queued', 'running', 'verifying', 'succeeded', 'failed', 'canceled', 'canceling'])
const PROBE_STATES = new Set(['PASS', 'FAIL', 'NOT_RUN', 'BLOCKED'])

function text(value, fallback = '') {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback
}

export function normalizeCapabilities(value = {}) {
  const runtime = normalizeRuntimeState(value)
  return {
    ...runtime,
    schemaVersion: value.schema_version || 'futures.capabilities.v1',
    products: Array.isArray(value.products) ? value.products : [],
    features: {
      research: Boolean(value.features?.research),
      tracking: Boolean(value.features?.tracking),
      export: Boolean(value.features?.export),
    },
    limits: value.limits || {},
  }
}

export function normalizeProduct(value = {}) {
  return {
    id: text(value.id),
    productId: text(value.product_id || value.id),
    name: text(value.name, text(value.id)),
    exchange: text(value.exchange),
    templateVersion: text(value.template_version),
    admission: text(value.admission, 'blocked'),
  }
}

export function normalizeContracts(value = {}) {
  return (value.items || []).map(item => ({
    id: text(item.id),
    productId: text(item.product_id),
    kind: ['actual', 'continuous', 'dominant'].includes(item.kind) ? item.kind : 'actual',
    lastTradingAt: text(item.last_trading_at),
    pricePrecision: Number.isInteger(item.price_precision) ? item.price_precision : 0,
    priceUnit: text(item.price_unit),
    multiplier: text(item.multiplier),
    calendarVersion: text(item.calendar_version),
    sourceVersion: text(item.source_version),
  }))
}

export function runDisplayState(run = {}) {
  const backend = text(run.status, 'unknown')
  if (!RUN_STATES.has(backend)) return { label: '状态未知', tone: 'blocked', terminal: false, serverState: backend }
  const labels = {
    queued: '排队中', running: '执行中', verifying: '核验中', canceling: '取消中',
    succeeded: '已完成', failed: '失败', canceled: '已取消',
  }
  return {
    label: labels[backend],
    tone: ['succeeded'].includes(backend) ? 'ready' : ['failed'].includes(backend) ? 'blocked' : 'warning',
    terminal: ['succeeded', 'failed', 'canceled'].includes(backend),
    serverState: backend,
  }
}

export function normalizeRun(value = {}) {
  const state = runDisplayState(value)
  return {
    id: text(value.id),
    draftId: text(value.draft_id),
    draftRevision: Number(value.draft_revision) || 0,
    status: state.serverState,
    displayState: state,
    stage: text(value.stage),
    asOf: text(value.as_of),
    horizonEnd: text(value.horizon_end),
    report: value.report || null,
    failureCode: text(value.failure_code),
    createdAt: text(value.created_at),
    traceId: text(value.trace_id),
  }
}

export function normalizeHypothesis(value = {}) {
  return {
    id: text(value.id),
    version: Number(value.version) || 1,
    runId: text(value.run_id),
    reportId: text(value.report_id),
    proposition: text(value.proposition),
    productId: text(value.product_id),
    contractId: text(value.contract_id),
    expiresAt: text(value.expires_at),
    lifecycle: text(value.lifecycle, 'draft'),
    userView: text(value.user_view, 'undetermined'),
    needsReview: Boolean(value.needs_review),
    conditions: Array.isArray(value.conditions) ? value.conditions : [],
    createdAt: text(value.created_at),
  }
}

export function normalizeNotification(value = {}) {
  return {
    id: text(value.id),
    type: text(value.type),
    objectId: text(value.object_id),
    objectVersion: Number(value.object_version) || 0,
    readAt: text(value.read_at, null),
    createdAt: text(value.created_at),
    severity: ['run_failed', 'source_unavailable', 'retraction'].includes(value.type) ? 'high' : 'normal',
  }
}

export function normalizeSource(value = {}) {
  return {
    id: text(value.id),
    version: Number(value.version) || 1,
    publisher: text(value.publisher),
    url: text(value.url),
    status: text(value.status, 'registered'),
    credentialRefConfigured: Boolean(value.credential_ref),
    rights: value.rights || {},
    metrics: Array.isArray(value.metrics) ? value.metrics : [],
    scheduleVersion: text(value.schedule_version),
    adapterVersion: text(value.adapter_version),
    coverageStart: text(value.coverage_start),
    coverageEnd: text(value.coverage_end),
    health: text(value.health, 'unknown'),
  }
}

export function normalizeSourceProbe(value = {}) {
  const result = PROBE_STATES.has(value.result) ? value.result : 'NOT_RUN'
  return {
    id: text(value.id),
    sourceId: text(value.source_id),
    sourceVersion: Number(value.source_version) || 0,
    adapterVersion: text(value.adapter_version),
    configDigest: text(value.config_digest),
    secretRevision: Number(value.secret_revision) || 0,
    secretFingerprint: text(value.secret_fingerprint),
    result,
    reason: text(value.reason),
    evidenceHash: text(value.evidence_hash),
    createdAt: text(value.created_at),
    releaseGateId: text(value.release_gate_id, null),
  }
}

export function canSubmitResearch({ capabilities, input, acceptedClaims, parseState }) {
  const blockingReasons = []
  if (capabilities?.blocking) blockingReasons.push(capabilities.reason || 'MODULE_NOT_READY')
  if (!input?.product_id || !input?.contract_id) blockingReasons.push('PRODUCT_AND_CONTRACT_REQUIRED')
  if (!Array.isArray(acceptedClaims) || acceptedClaims.length === 0) blockingReasons.push('CLAIMS_REQUIRED')
  if (acceptedClaims?.length > 12) blockingReasons.push('TOO_MANY_CLAIMS')
  if (parseState && parseState !== 'succeeded') blockingReasons.push('PARSE_NOT_SUCCEEDED')
  return { allowed: blockingReasons.length === 0, blockingReasons }
}

export function notificationMatchesFilter(notification, filter = {}) {
  if (filter.type && notification.type !== filter.type) return false
  if (filter.objectId && notification.objectId !== filter.objectId) return false
  if (filter.objectVersion && notification.objectVersion !== Number(filter.objectVersion)) return false
  if (filter.read === true && !notification.readAt) return false
  if (filter.read === false && notification.readAt) return false
  return true
}
