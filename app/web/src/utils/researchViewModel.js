import { STAGE_A_PUBLIC_MODE } from '../config/publicMode.js'
import {
  CLAIM_TYPE_LABELS,
  STATUS_MESSAGES,
  VERDICT_LABELS,
  emptyField,
  formatDateTime,
  historyTitle,
  modeLabel,
  statusLabel
} from './researchCopy.js'

function asList(value) {
  return Array.isArray(value) ? value : []
}

export function instrumentLabel(item) {
  if (!item) return ''
  const name = item.name || ''
  const symbol = item.symbol || ''
  const id = item.instrument_id || item.id || ''
  const market = item.market === 'HK' ? ' · 港股' : item.market === 'A' ? ' · A股' : ''
  if (name && symbol) return `${name} ${symbol}${market}`
  if (name && id) return `${name} ${id}${market}`
  return (name || symbol || id) + market
}

export function toEvidenceLineModel(report) {
  if (!report) {
    return {
      published: false,
      partial: false,
      supportCount: null,
      challengeCount: null,
      unknownCount: null,
      dualUse: false,
      summary: '证据整理中，发布后显示可核查来源'
    }
  }
  const allowed = new Set(asList(report.evidence_ids))
  const supportIDs = new Set()
  const challengeIDs = new Set()
  for (const arg of asList(report.support)) {
    for (const id of asList(arg.evidence_ids)) {
      if (allowed.has(id)) supportIDs.add(id)
    }
  }
  for (const arg of asList(report.challenge)) {
    for (const id of asList(arg.evidence_ids)) {
      if (allowed.has(id)) challengeIDs.add(id)
    }
  }
  const dualUse = [...supportIDs].some((id) => challengeIDs.has(id))
  const supportCount = supportIDs.size
  const challengeCount = challengeIDs.size
  const unknownCount = asList(report.unknowns).length
  return {
    published: true,
    partial: report.quality_status === 'incomplete',
    supportCount,
    challengeCount,
    unknownCount,
    dualUse,
    summary: `支持来源 ${supportCount} / 挑战来源 ${challengeCount} / 未知项 ${unknownCount}`
  }
}

export function evidenceRelation(report, evidenceId) {
  if (!report || !evidenceId) return ''
  const allowed = new Set(asList(report.evidence_ids))
  if (!allowed.has(evidenceId)) return ''
  const inSupport = asList(report.support).some((arg) => asList(arg.evidence_ids).includes(evidenceId))
  const inChallenge = asList(report.challenge).some((arg) => asList(arg.evidence_ids).includes(evidenceId))
  if (inSupport && inChallenge) return 'both'
  if (inSupport) return 'support'
  if (inChallenge) return 'challenge'
  return ''
}

export function evidenceIndexMap(report) {
  const map = {}
  asList(report?.evidence_ids).forEach((id, i) => {
    map[id] = i + 1
  })
  return map
}

export function toHistoryRowModel(item, instruments = []) {
  const match = instruments.find((row) => row.instrument_id === item?.instrument_id)
  return {
    run_id: item?.run_id || '',
    status: item?.status || '',
    statusText: statusLabel(item?.status),
    instrument_id: item?.instrument_id || '',
    instrument_name: match?.name || '',
    title: historyTitle({
      instrument_id: item?.instrument_id,
      instrument_name: match?.name
    }),
    mode: item?.mode || '',
    modeText: modeLabel(item?.mode) || '模式未确认',
    created_at: item?.created_at,
    createdText: formatDateTime(item?.created_at),
    deleting: !!item?.deleting
  }
}

export function toResearchViewModel(runView, instruments = []) {
  if (!runView) return null
  const match = instruments.find((row) => row.instrument_id === runView.instrument_id)
  const report = runView.report || null
  const status = runView.status || ''
  const known = Object.prototype.hasOwnProperty.call(STATUS_MESSAGES, status)
  const verdict = report?.verdict || null
  const quality = report?.quality_status || ''
  return {
    runId: runView.run_id,
    status,
    knownStatus: known,
    statusMessage: known ? STATUS_MESSAGES[status] : '暂无法识别研究状态',
    stage: runView.stage || '',
    mode: runView.mode || '',
    modeText: modeLabel(runView.mode) || '暂未提供',
    asOf: runView.as_of,
    asOfText: formatDateTime(runView.as_of),
    updatedAt: runView.updated_at,
    updatedText: formatDateTime(runView.updated_at),
    instrumentId: runView.instrument_id || '',
    instrumentName: match?.name || '',
    instrumentText: match ? instrumentLabel(match) : emptyField(runView.instrument_id),
    horizon: runView.horizon || runView.claim?.horizon || '',
    claimText: runView.claim?.text || '',
    report,
    quality,
    verdict,
    verdictText: verdict ? (VERDICT_LABELS[verdict] || '') : '',
    warnings: asList(runView.warnings),
    taskError: runView.error || null,
    evidenceLine: toEvidenceLineModel(report),
    indexMap: evidenceIndexMap(report),
    allowedIds: asList(report?.evidence_ids)
  }
}

export function stageRailModel(status, report) {
  const nodes = [
    { key: 'queued', label: '排队' },
    { key: 'researching', label: '调查' },
    { key: 'verifying', label: '核对' },
    { key: 'report', label: '报告' }
  ]
  const order = { queued: 0, researching: 1, verifying: 2 }
  const currentIndex = order[status] ?? -1
  const published = !!report
  const failedLike = ['failed', 'canceled', 'incomplete'].includes(status)
  const canceling = status === 'canceling'
  const completed = status === 'completed' && published
  return nodes.map((node, index) => {
    let state = 'pending'
    if (node.key === 'report') {
      if (published && (completed || status === 'incomplete')) state = status === 'incomplete' ? 'partial' : 'done'
      else if (status === 'completed' && !published) state = 'pending'
      else if (['verifying', 'researching', 'queued'].includes(status) || canceling) state = 'pending'
    } else if (completed) {
      state = 'done'
    } else if (status === node.key) {
      state = 'current'
    } else if (currentIndex > index) {
      state = 'done'
    } else if (failedLike || canceling) {
      state = currentIndex > index ? 'done' : 'pending'
    }
    if ((failedLike || canceling) && node.key === 'report') state = published ? (status === 'incomplete' ? 'partial' : 'done') : 'pending'
    return { ...node, state }
  })
}

export function claimTypeLabel(type) {
  return CLAIM_TYPE_LABELS[type] || '主张'
}
