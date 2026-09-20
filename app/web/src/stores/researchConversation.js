import { defineStore } from 'pinia'
import {
  askQuestion,
  cancelResearch,
  createResearch,
  deleteResearch,
  getEvidence,
  getResearch,
  listResearch,
  parseClaim
} from '../api/research.js'
import { buildChats } from '../components/research/chatAdapter.js'
import {
  ACTIVE_STATUSES,
  countChars,
  mapRequestError
} from '../utils/researchCopy.js'

let pollTimer = null
let nextId = 1
function uid(prefix) {
  nextId += 1
  return `${prefix}-${Date.now()}-${nextId}`
}

function now() {
  return Date.now()
}

export const useResearchConversation = defineStore('researchConversation', {
  state: () => ({
    draftText: '',
    parseBusy: false,
    parseError: '',
    draft: null,
    parsedText: '',
    instrumentId: '',
    horizon: '',
    confirmBusy: false,
    confirmError: '',
    parentRunId: '',
    userMessage: null,
    confirmMessage: null,
    notice: null,
    currentRunId: '',
    runView: null,
    runError: '',
    followups: [],
    questionBusy: false,
    questionError: '',
    evidenceOpen: false,
    evidence: null,
    evidenceError: '',
    evidenceSourceEl: null,
    historyOpen: false,
    composing: false,
    lastEvidenceButton: null,
    activeConflictId: ''
  }),
  getters: {
    chats: (state) => buildChats(state),
    charCount: (state) => countChars(state.draftText),
    staleDraft: (state) => !!state.draft && state.draftText.trim() !== state.parsedText.trim(),
    canParse: (state) => {
      const n = countChars(state.draftText)
      return n >= 20 && n <= 2000 && !state.parseBusy && !state.confirmBusy
    },
    canStart: (state) => !!state.draft && !!state.instrumentId && !!state.horizon.trim() && !state.confirmBusy && !state.parseBusy && state.draftText.trim() === state.parsedText.trim(),
    isActiveRun: (state) => ACTIVE_STATUSES.includes(state.runView?.status),
    hasPublishedReport: (state) => !!state.runView?.report,
    followupCount: (state) => state.followups.length,
    followupLimited: (state) => state.followups.length >= 3,
    composerMode: (state) => {
      if (ACTIVE_STATUSES.includes(state.runView?.status)) return 'busy'
      if (state.runView?.report) {
        if (state.followups.length >= 3) return 'followup_limit'
        return 'followup'
      }
      if (state.currentRunId && state.runView && !state.runView.report) return 'locked'
      return 'claim'
    }
  },
  actions: {
    resetAll() {
      this.stopPolling()
      this.draftText = ''
      this.parseBusy = false
      this.parseError = ''
      this.draft = null
      this.parsedText = ''
      this.instrumentId = ''
      this.horizon = ''
      this.confirmBusy = false
      this.confirmError = ''
      this.parentRunId = ''
      this.userMessage = null
      this.confirmMessage = null
      this.notice = null
      this.currentRunId = ''
      this.runView = null
      this.runError = ''
      this.followups = []
      this.questionBusy = false
      this.questionError = ''
      this.closeEvidence()
      this.historyOpen = false
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
    },
    setDraftText(text) {
      this.draftText = text
      this.parseError = ''
      if (this.draft && text.trim() !== this.parsedText.trim()) {
        this.draft = null
        this.instrumentId = ''
        this.horizon = ''
        this.confirmMessage = null
      }
    },
    fillExample(text) {
      this.setDraftText(text)
    },
    setInstrument(id) {
      this.instrumentId = id
    },
    setHorizon(value) {
      this.horizon = value
    },
    async parseCurrent() {
      if (!this.canParse) return
      this.parseError = ''
      this.parseBusy = true
      const snapshot = this.draftText
      try {
        const res = await parseClaim(snapshot)
        this.draft = res.data
        this.parsedText = snapshot
        this.instrumentId = res.data.candidates?.[0]?.instrument_id || ''
        this.horizon = res.data.suggested_horizon || ''
        this.userMessage = { id: uid('claim'), text: snapshot, createAt: now() }
        this.confirmMessage = { id: uid('confirm'), createAt: now() }
        this.notice = null
      } catch (error) {
        this.parseError = mapRequestError(error, '解析失败，原文已保留，可重试')
      } finally {
        this.parseBusy = false
      }
    },
    async startResearch() {
      if (!this.canStart) return null
      this.confirmError = ''
      this.confirmBusy = true
      try {
        const key = crypto.randomUUID()
        const res = await createResearch({
          draft_id: this.draft.draft_id,
          revision: this.draft.revision,
          instrument_id: this.instrumentId,
          horizon: this.horizon,
          as_of: new Date().toISOString(),
          parent_run_id: this.parentRunId || '',
          claim_text: this.draftText
        }, key)
        this.currentRunId = res.data.run_id
        this.confirmError = ''
        await this.loadRun(res.data.run_id, { keepLocal: true })
        return res.data.run_id
      } catch (error) {
        this.confirmError = mapRequestError(error, '无法开始研究，原文已保留')
        if (error?.code === 'ACTIVE_RUN_EXISTS') {
          const activeId = await this.findActiveRunId()
          if (activeId) this.confirmError = `已有研究进行中，可返回继续查看。`
          this.activeConflictId = activeId
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
      if (!keepLocal && this.currentRunId !== id) {
        this.followups = []
        this.questionError = ''
        this.confirmMessage = null
        this.draft = null
        this.notice = null
      }
      this.currentRunId = id
      try {
        const res = await getResearch(id)
        this.runView = res.data
        this.runError = ''
        if (!keepLocal) {
          const text = res.data.claim?.text || ''
          if (text) {
            this.userMessage = this.userMessage?.text === text
              ? this.userMessage
              : { id: `claim-${id}`, text, createAt: now() }
            this.draftText = this.draftText || text
          }
          this.instrumentId = res.data.instrument_id || this.instrumentId
          this.horizon = res.data.horizon || this.horizon
        }
        if (this.isActiveRun) this.startPolling()
        else this.stopPolling()
      } catch (error) {
        this.runError = mapRequestError(error, '连接已中断，研究状态可能仍在更新')
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
      if (!this.currentRunId) return
      try {
        await cancelResearch(this.currentRunId)
        await this.loadRun(this.currentRunId, { keepLocal: true })
      } catch (error) {
        this.runError = mapRequestError(error, '取消请求失败，可重试')
      }
    },
    async deleteCurrent() {
      if (!this.currentRunId) return false
      try {
        await deleteResearch(this.currentRunId)
        this.resetAll()
        return true
      } catch (error) {
        this.runError = mapRequestError(error, '删除失败，可重试')
        return false
      }
    },
    async askFollowup(text) {
      const value = (text || '').trim()
      if (!this.hasPublishedReport || this.followupLimited || !value || this.questionBusy) return
      this.questionBusy = true
      this.questionError = ''
      const questionId = uid('q')
      const item = { questionId, answerId: uid('a'), text: value, createAt: now(), answer: null }
      this.followups = this.followups.concat(item)
      try {
        const res = await askQuestion(this.currentRunId, value, crypto.randomUUID())
        this.followups = this.followups.map((row) => row.questionId === questionId ? { ...row, answer: res.data } : row)
      } catch (error) {
        this.followups = this.followups.filter((row) => row.questionId !== questionId)
        this.questionError = mapRequestError(error, '追问失败，原文已保留')
        this.draftText = value
      } finally {
        this.questionBusy = false
      }
    },
    async openEvidence(evidenceId, sourceEl) {
      const allowed = new Set(this.runView?.report?.evidence_ids || [])
      if (!allowed.has(evidenceId)) return
      this.historyOpen = false
      this.evidenceSourceEl = sourceEl || document.activeElement
      this.evidenceError = ''
      this.evidenceOpen = true
      this.evidence = null
      try {
        const res = await getEvidence(evidenceId)
        this.evidence = res.data
      } catch (error) {
        this.evidenceError = mapRequestError(error, '证据加载失败，可重试')
      }
    },
    closeEvidence() {
      this.evidenceOpen = false
      this.evidence = null
      this.evidenceError = ''
      const el = this.evidenceSourceEl
      this.evidenceSourceEl = null
      if (el && typeof el.focus === 'function') {
        requestAnimationFrame(() => el.focus())
      }
    },
    retryEvidence() {
      const id = this.evidence?.evidence_id
      if (id) this.openEvidence(id, this.evidenceSourceEl)
    },
    openHistory() {
      this.evidenceOpen = false
      this.historyOpen = true
    },
    closeHistory() {
      this.historyOpen = false
    }
  }
})
