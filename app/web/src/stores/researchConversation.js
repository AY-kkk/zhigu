import { defineStore } from 'pinia'
import {
  askQuestion,
  cancelResearch,
  createResearch,
  deleteResearch,
  getEvidence,
  getResearch,
  listInstruments,
  listResearch,
  parseClaim,
  patchClaim,
  uploadResearchDocument
} from '../api/research.js'
import { STAGE_A_PUBLIC_MODE } from '../config/publicMode.js'
import {
  ACTIVE_STATUSES,
  CANCELABLE_STATUSES,
  countChars,
  mapRequestError
} from '../utils/researchCopy.js'
import { evidenceRelation } from '../utils/researchViewModel.js'

let pollTimer = null
let nextId = 1
function uid(prefix) {
  nextId += 1
  return `${prefix}-${Date.now()}-${nextId}`
}
function now() {
  return Date.now()
}
function fingerprintCreate(payload) {
  return JSON.stringify({
    draft_id: payload.draft_id,
    revision: payload.revision,
    parent_run_id: payload.parent_run_id || ''
  })
}

function splitHorizon(horizon) {
  const parts = String(horizon || '').split('/').map((s) => s.trim())
  if (parts.length === 2 && parts[0] && parts[1]) {
    return { start: parts[0], end: parts[1] }
  }
  return { start: '', end: '' }
}

export const useResearchConversation = defineStore('researchConversation', {
  state: () => ({
    draftText: '',
    focusText: '',
    document: null,
    documentFile: null,
    documentUploading: false,
    documentError: '',
    parseBusy: false,
    parseError: '',
    parseSeq: 0,
    draft: null,
    parsedText: '',
    parsedDocumentId: '',
    parsedFocusText: '',
    instrumentId: '',
    horizon: '',
    confirmBusy: false,
    confirmError: '',
    createAttempt: null,
    parentRunId: '',
    userMessage: null,
    notice: null,
    currentRunId: '',
    runView: null,
    runError: '',
    runLoading: false,
    disconnected: false,
    loadSeq: 0,
    followups: [],
    questionBusy: false,
    questionError: '',
    questionLocked: false,
    questionAttempt: null,
    evidenceOpen: false,
    evidence: null,
    evidenceError: '',
    evidenceBusy: false,
    requestedEvidenceId: '',
    evidenceRelation: '',
    evidenceSeq: 0,
    evidenceSourceEl: null,
    composing: false,
    activeConflictId: '',
    instruments: [],
    catalogReady: false,
    dataMode: STAGE_A_PUBLIC_MODE,
    composerFocusToken: 0
  }),
  getters: {
    charCount: (state) => countChars(state.draftText),
    staleDraft: (state) => !!state.draft && (
      state.draftText.trim() !== state.parsedText.trim()
      || (state.document?.document_id || '') !== state.parsedDocumentId
      || state.focusText.trim() !== state.parsedFocusText.trim()
    ),
    inputMode: (state) => state.draft?.input_mode || (
      state.draftText.trim() && state.document ? 'claim_and_report'
        : state.document ? 'report_only' : 'claim_only'
    ),
    canParse: (state) => {
      const n = countChars(state.draftText)
      const textOK = n === 0 || (n >= 20 && n <= 2000)
      return textOK && (n > 0 || !!state.document?.document_id) && !state.parseBusy && !state.confirmBusy && !state.documentUploading
    },
    canStart: (state) => {
      const stale = !!state.draft && (
        state.draftText.trim() !== state.parsedText.trim()
        || (state.document?.document_id || '') !== state.parsedDocumentId
        || state.focusText.trim() !== state.parsedFocusText.trim()
      )
      return !!state.draft
        && !!state.instrumentId
        && !!state.horizon.trim()
        && !state.confirmBusy
        && !state.parseBusy
        && !stale
    },
    isActiveRun: (state) => ACTIVE_STATUSES.includes(state.runView?.status),
    hasPublishedReport: (state) => !!state.runView?.report,
    followupCount: (state) => state.followups.filter((row) => row.answer).length,
    followupLimited: (state) => state.questionLocked || state.followups.filter((row) => row.answer).length >= 3,
    displayMode: (state) => state.runView?.mode || state.draft?.mode || state.dataMode || STAGE_A_PUBLIC_MODE,
    composerMode: (state) => {
      if (ACTIVE_STATUSES.includes(state.runView?.status)) return 'busy'
      if (state.runView?.report) {
        if (state.questionLocked || state.followups.filter((row) => row.answer).length >= 3) return 'followup_limit'
        return 'followup'
      }
      if (state.currentRunId && state.runView && !state.runView.report) return 'locked'
      return 'claim'
    }
  },
  actions: {
    resetAll() {
      this.stopPolling()
      this.parseSeq += 1
      this.loadSeq += 1
      this.evidenceSeq += 1
      this.draftText = ''
      this.focusText = ''
      this.document = null
      this.documentFile = null
      this.documentUploading = false
      this.documentError = ''
      this.parseBusy = false
      this.parseError = ''
      this.draft = null
      this.parsedText = ''
      this.parsedDocumentId = ''
      this.parsedFocusText = ''
      this.instrumentId = ''
      this.horizon = ''
      this.confirmBusy = false
      this.confirmError = ''
      this.createAttempt = null
      this.parentRunId = ''
      this.userMessage = null
      this.notice = null
      this.currentRunId = ''
      this.runView = null
      this.runError = ''
      this.runLoading = false
      this.disconnected = false
      this.followups = []
      this.questionBusy = false
      this.questionError = ''
      this.questionLocked = false
      this.questionAttempt = null
      this.closeEvidence()
      this.activeConflictId = ''
    },
    prepareNew(query = {}) {
      const parent = typeof query.parent_run_id === 'string' ? query.parent_run_id : ''
      if (parent && parent !== this.parentRunId) {
        this.resetAll()
        this.parentRunId = parent
        this.notice = {
          id: uid('notice'),
          text: '重新研究将创建新任务，不会覆盖旧报告。',
          createAt: now()
        }
      }
      if (!parent) this.parentRunId = this.parentRunId || ''
      this.ensureInstruments()
    },
    setDraftText(text) {
      this.draftText = text
      this.parseError = ''
    },
    setFocusText(text) {
      this.focusText = text
      this.parseError = ''
    },
    async uploadDocument(file) {
      this.documentError = ''
      this.documentUploading = true
      try {
        const res = await uploadResearchDocument(file)
        this.document = res.data
        this.documentFile = file
        this.parseError = ''
      } catch (error) {
        this.document = null
        this.documentFile = null
        this.documentError = mapRequestError(error, '研报上传失败，可重试')
      } finally {
        this.documentUploading = false
      }
    },
    removeDocument() {
      this.document = null
      this.documentFile = null
      this.documentError = ''
    },
    fillExample(text) {
      this.setDraftText(text)
    },
    requestComposerFocus() {
      this.composerFocusToken += 1
    },
    setInstrument(id) {
      this.instrumentId = id
    },
    mergeInstruments(items) {
      const map = new Map(this.instruments.map((row) => [row.instrument_id, row]))
      for (const row of items || []) {
        if (row?.instrument_id) map.set(row.instrument_id, row)
      }
      this.instruments = [...map.values()]
    },
    async searchInstruments(q) {
      try {
        const res = await listInstruments({ q, limit: 20 })
        const items = res.data.items || []
        this.mergeInstruments(items)
        return items
      } catch {
        return []
      }
    },
    setHorizon(value) {
      this.horizon = value
    },
    async ensureInstruments() {
      if (this.catalogReady) return
      try {
        const res = await listInstruments()
        this.mergeInstruments(res.data.items || [])
        if (res.data.mode) this.dataMode = res.data.mode
        this.catalogReady = true
      } catch {
        this.instruments = this.instruments || []
      }
    },
    async parseCurrent() {
      if (!this.canParse) return
      this.parseError = ''
      this.parseBusy = true
      this.parseSeq += 1
      const seq = this.parseSeq
      const snapshot = this.draftText
      const documentId = this.document?.document_id || ''
      const focus = this.focusText.trim()
      const display = snapshot || this.document?.filename || '上传研报'
      this.userMessage = { id: uid('claim'), text: display, createAt: now() }
      try {
        const res = await parseClaim({ text: snapshot, document_id: documentId, focus_text: focus })
        if (seq !== this.parseSeq) return
        this.draft = res.data
        this.parsedText = snapshot
        this.parsedDocumentId = documentId
        this.parsedFocusText = focus
        const candidates = res.data.candidates || []
        this.instrumentId = candidates.length === 1 ? candidates[0].instrument_id : this.instrumentId
        this.mergeInstruments(candidates)
        if (res.data.horizon_start && res.data.horizon_end) {
          this.horizon = `${res.data.horizon_start}/${res.data.horizon_end}`
        } else {
          this.horizon = res.data.suggested_horizon || ''
        }
        await this.ensureInstruments()
        this.notice = null
        this.createAttempt = null
      } catch (error) {
        if (error?.code === 'ERR_CANCELED' || error?.name === 'CanceledError') return
        if (seq !== this.parseSeq) return
        this.parseError = mapRequestError(error, '解析失败，原文已保留，可重试')
      } finally {
        if (seq === this.parseSeq) this.parseBusy = false
      }
    },
    async startResearch() {
      if (!this.canStart) return null
      this.confirmError = ''
      this.confirmBusy = true
      try {
        const dates = splitHorizon(this.horizon)
        const patched = await patchClaim(this.draft.draft_id, {
          revision: this.draft.revision,
          instrument_id: this.instrumentId,
          horizon_start: dates.start || undefined,
          horizon_end: dates.end || undefined,
          items: this.draft.items || []
        })
        this.draft = patched.data
        const payloadBase = {
          draft_id: this.draft.draft_id,
          revision: this.draft.revision,
          parent_run_id: this.parentRunId || ''
        }
        const mark = fingerprintCreate(payloadBase)
        if (!this.createAttempt || this.createAttempt.fingerprint !== mark) {
          this.createAttempt = {
            fingerprint: mark,
            key: crypto.randomUUID(),
            payload: payloadBase
          }
        }
        const res = await createResearch(this.createAttempt.payload, this.createAttempt.key)
        this.currentRunId = res.data.run_id
        this.confirmError = ''
        this.createAttempt = null
        await this.loadRun(res.data.run_id, { keepLocal: true })
        return res.data.run_id
      } catch (error) {
        this.confirmError = mapRequestError(error, '无法开始研究，原文已保留')
        if (error?.code === 'ACTIVE_RUN_EXISTS') {
          const activeId = await this.findActiveRunId()
          this.activeConflictId = activeId
          if (activeId) this.confirmError = '已有研究进行中，可返回继续查看。'
        }
        return null
      } finally {
        this.confirmBusy = false
      }
    },
    async findActiveRunId() {
      try {
        const res = await listResearch({ limit: 20 })
        const hit = (res.data.items || []).find((item) => ACTIVE_STATUSES.includes(item.status))
        return hit?.run_id || ''
      } catch {
        return ''
      }
    },
    async loadRun(id, { keepLocal = false } = {}) {
      if (!id) return
      this.loadSeq += 1
      const seq = this.loadSeq
      if (!keepLocal && this.currentRunId !== id) {
        this.followups = []
        this.questionError = ''
        this.questionLocked = false
        this.questionAttempt = null
        this.draft = null
        this.notice = null
        this.runView = null
        this.runError = ''
        this.disconnected = false
        this.closeEvidence()
      }
      this.currentRunId = id
      if (!this.runView) this.runLoading = true
      try {
        const res = await getResearch(id)
        if (seq !== this.loadSeq || this.currentRunId !== id) return
        this.runView = res.data
        this.runError = ''
        this.disconnected = false
        this.runLoading = false
        if (!keepLocal) {
          this.draftText = ''
      this.focusText = ''
      this.document = null
      this.documentFile = null
      this.documentUploading = false
      this.documentError = ''
          const text = res.data.claim?.text || ''
          if (text) {
            this.userMessage = this.userMessage?.text === text
              ? this.userMessage
              : { id: `claim-${id}`, text, createAt: now() }
          }
          this.instrumentId = res.data.instrument_id || this.instrumentId
          this.horizon = res.data.horizon || this.horizon
        }
        if (this.isActiveRun) this.startPolling()
        else this.stopPolling()
      } catch (error) {
        if (error?.code === 'ERR_CANCELED' || error?.name === 'CanceledError') return
        if (seq !== this.loadSeq || this.currentRunId !== id) return
        this.runLoading = false
        const mapped = mapRequestError(error, '连接中断，显示上次状态')
        if (this.runView && this.runView.run_id === id) {
          this.disconnected = true
          this.runError = '连接中断，显示上次状态'
        } else {
          this.runError = mapped
        }
      }
    },
    startPolling() {
      this.stopPolling()
      pollTimer = setInterval(() => {
        if (this.currentRunId) this.loadRun(this.currentRunId, { keepLocal: true })
      }, 2000)
    },
    stopPolling() {
      if (pollTimer) {
        clearInterval(pollTimer)
        pollTimer = null
      }
    },
    async cancelCurrent() {
      if (!this.currentRunId || !CANCELABLE_STATUSES.includes(this.runView?.status)) return
      try {
        await cancelResearch(this.currentRunId)
        await this.loadRun(this.currentRunId, { keepLocal: true })
      } catch (error) {
        this.runError = mapRequestError(error, '取消请求失败，可重试')
      }
    },
    async deleteRun(id) {
      if (!id) return { ok: false, status: '' }
      try {
        const res = await deleteResearch(id)
        const status = res.data?.deletion_status || 'scheduled'
        if (id === this.currentRunId) this.resetAll()
        return { ok: true, status }
      } catch (error) {
        return { ok: false, status: '', error: mapRequestError(error, '删除失败，可重试') }
      }
    },
    async deleteCurrent() {
      const result = await this.deleteRun(this.currentRunId)
      return result.ok
    },
    async askFollowup(text) {
      const value = (text || '').trim()
      if (!this.hasPublishedReport || this.followupLimited || !value || this.questionBusy) return
      if (countChars(value) > 2000) return
      this.questionBusy = true
      this.questionError = ''
      if (!this.questionAttempt || this.questionAttempt.text !== value) {
        this.questionAttempt = { text: value, key: crypto.randomUUID() }
      }
      const questionId = uid('q')
      const item = { questionId, answerId: uid('a'), text: value, createAt: now(), answer: null }
      this.followups = this.followups.concat(item)
      try {
        const res = await askQuestion(this.currentRunId, value, this.questionAttempt.key)
        this.followups = this.followups.map((row) => (
          row.questionId === questionId ? { ...row, answer: res.data } : row
        ))
        this.questionAttempt = null
      } catch (error) {
        this.followups = this.followups.filter((row) => row.questionId !== questionId)
        this.questionError = mapRequestError(error, '追问失败，原文已保留')
        this.draftText = value
        if (error?.code === 'QUESTION_LIMIT') this.questionLocked = true
      } finally {
        this.questionBusy = false
      }
    },
    async openEvidence(evidenceId, sourceEl) {
      const allowed = new Set(this.runView?.report?.evidence_ids || [])
      if (!allowed.has(evidenceId)) return
      const runId = this.currentRunId
      this.evidenceSeq += 1
      const seq = this.evidenceSeq
      this.requestedEvidenceId = evidenceId
      this.evidenceRelation = evidenceRelation(this.runView?.report, evidenceId)
      this.evidenceSourceEl = sourceEl || document.activeElement
      this.evidenceError = ''
      this.evidence = null
      this.evidenceBusy = true
      this.evidenceOpen = true
      try {
        const res = await getEvidence(evidenceId)
        if (seq !== this.evidenceSeq || this.requestedEvidenceId !== evidenceId || this.currentRunId !== runId) return
        this.evidence = res.data
        this.evidenceBusy = false
      } catch (error) {
        if (error?.code === 'ERR_CANCELED' || error?.name === 'CanceledError') return
        if (seq !== this.evidenceSeq || this.requestedEvidenceId !== evidenceId) return
        this.evidenceBusy = false
        this.evidenceError = mapRequestError(error, '证据加载失败，可重试')
      }
    },
    closeEvidence() {
      this.evidenceSeq += 1
      this.evidenceOpen = false
      this.evidence = null
      this.evidenceError = ''
      this.evidenceBusy = false
      this.requestedEvidenceId = ''
      this.evidenceRelation = ''
      const el = this.evidenceSourceEl
      this.evidenceSourceEl = null
      if (el && typeof el.focus === 'function') {
        requestAnimationFrame(() => el.focus())
      }
    },
    retryEvidence() {
      const id = this.requestedEvidenceId
      if (id) this.openEvidence(id, this.evidenceSourceEl)
    }
  }
})
