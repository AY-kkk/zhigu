import { defineStore } from 'pinia'
import * as api from '../api/intel.js'
import { isDemoExpired } from '../utils/intelHttp.js'

function key() { return `intel-${Math.random().toString(36).slice(2)}-${Date.now()}` }
function errText(error) { return error?.message || '请求失败' }

export const useIntelWorkspace = defineStore('intelWorkspace', {
  state: () => ({
    mode: 'live',
    accountEpoch: 0,
    session: null,
    bootBusy: false,
    bootError: '',
    demoNeedsStart: false,
    instruments: [],
    instrumentQuery: '',
    watchlist: [],
    events: [],
    eventFilters: { code: '', type: '', verification: '' },
    eventsBusy: false,
    eventsError: '',
    eventsSeq: 0,
    eventsAbort: null,
    selectedEvent: null,
    selectedBusy: false,
    selectedError: '',
    timeline: [],
    evidence: [],
    conflicts: [],
    changes: [],
    notifications: [],
    notificationsBusy: false,
    notificationsError: '',
    notificationsSeq: 0,
    notificationsAbort: null,
    unreadCount: 0,
    dataStatus: null,
    replay: { busy: false, error: '', result: null, key: '', fingerprint: '' },
    admin: { reviews: [], reviewsBusy: false, reviewsError: '', jobs: [], importError: '', busy: false, sourceForm: { provider: 'fixture', publisher: 'DEMO 人工导入', document_id: '', title: '', text: '', rights: 'summary', import_reason: '' }, jobForm: { provider: 'fixture', codes: 'DEMO.A', from: '', to: '' } },
    activeTab: 'events'
  }),
  actions: {
    resetForAccount(mode = this.mode) {
      this.eventsAbort?.abort()
      this.notificationsAbort?.abort()
      const nextEpoch = this.accountEpoch + 1
      this.$reset()
      this.accountEpoch = nextEpoch
      this.mode = mode
    },
    async bootstrap(mode = this.mode) {
      const epoch = this.accountEpoch
      this.mode = mode
      this.bootBusy = true
      this.bootError = ''
      this.demoNeedsStart = mode === 'demo'
      try {
        try {
          const result = await api.getSession(mode)
          if (epoch !== this.accountEpoch) return
          this.session = result.data
          this.demoNeedsStart = false
        } catch (error) {
          if (epoch !== this.accountEpoch) return
          if (mode === 'demo' && isDemoExpired(error)) {
            this.demoNeedsStart = true
            return
          }
          throw error
        }
        await this.loadWorkspace(epoch)
        if (this.mode === 'live' && localStorage.getItem('zhigu_role') === 'admin') await this.fetchReviews(epoch)
      } catch (error) {
        if (epoch === this.accountEpoch) this.bootError = errText(error)
      } finally {
        if (epoch === this.accountEpoch) this.bootBusy = false
      }
    },
    async startDemo() {
      const epoch = this.accountEpoch
      this.bootBusy = true
      this.bootError = ''
      try {
        const result = await api.createDemoSession(key())
        if (epoch !== this.accountEpoch) return
        this.session = result.data
        this.demoNeedsStart = false
        await this.loadWorkspace(epoch)
        if (this.mode === 'live' && localStorage.getItem('zhigu_role') === 'admin') await this.fetchReviews(epoch)
      } catch (error) {
        if (epoch === this.accountEpoch) this.bootError = errText(error)
      } finally {
        if (epoch === this.accountEpoch) this.bootBusy = false
      }
    },
    async loadWorkspace(epoch = this.accountEpoch) {
      await Promise.all([
        this.fetchInstruments(epoch),
        this.fetchWatchlist(epoch),
        this.fetchEvents(epoch),
        this.fetchNotifications(epoch),
        this.fetchDataStatus(epoch)
      ])
    },
    async fetchInstruments(epoch = this.accountEpoch) {
      const result = await api.searchInstruments(this.mode, this.instrumentQuery, 20)
      if (epoch === this.accountEpoch) this.instruments = result.data?.items || []
    },
    async searchInstruments(q) {
      this.instrumentQuery = q
      await this.fetchInstruments()
    },
    async fetchWatchlist(epoch = this.accountEpoch) {
      const result = await api.getWatchlist(this.mode)
      if (epoch === this.accountEpoch) this.watchlist = result.data?.items || []
    },
    async addWatchlist(code) {
      const epoch = this.accountEpoch
      await api.addWatchlist(this.mode, code)
      await Promise.all([this.fetchWatchlist(epoch), this.fetchEvents(epoch)])
    },
    async removeWatchlist(code) {
      const epoch = this.accountEpoch
      await api.removeWatchlist(this.mode, code)
      await Promise.all([this.fetchWatchlist(epoch), this.fetchEvents(epoch)])
    },
    async fetchEvents(epoch = this.accountEpoch) {
      this.eventsSeq += 1
      const seq = this.eventsSeq
      this.eventsAbort?.abort()
      const abort = new AbortController()
      this.eventsAbort = abort
      this.eventsBusy = true
      this.eventsError = ''
      try {
        const result = await api.listEvents(this.mode, { ...this.eventFilters, limit: 50 }, { signal: abort.signal })
        if (seq !== this.eventsSeq || epoch !== this.accountEpoch) return
        this.events = result.data?.items || []
      } catch (error) {
        if (seq !== this.eventsSeq || epoch !== this.accountEpoch || error?.code === 'ABORTED') return
        this.eventsError = errText(error)
        this.events = []
      } finally {
        if (seq === this.eventsSeq) this.eventsBusy = false
      }
    },
    setEventFilter(name, value) {
      this.eventFilters[name] = value
      this.fetchEvents()
    },
    async selectEvent(eventId) {
      const epoch = this.accountEpoch
      this.selectedBusy = true
      this.selectedError = ''
      try {
        const [detail, timeline, evidence, conflicts, changes] = await Promise.all([
          api.getEvent(this.mode, eventId),
          api.getTimeline(this.mode, eventId),
          api.getEvidence(this.mode, eventId, { include_inactive: 'true' }),
          api.getConflicts(this.mode, eventId),
          api.getChanges(this.mode, eventId)
        ])
        if (epoch !== this.accountEpoch) return
        this.selectedEvent = detail.data
        this.timeline = timeline.data?.items || []
        this.evidence = evidence.data?.items || []
        this.conflicts = conflicts.data?.items || []
        this.changes = changes.data?.items || []
        this.activeTab = 'detail'
      } catch (error) {
        if (epoch === this.accountEpoch) this.selectedError = errText(error)
      } finally {
        if (epoch === this.accountEpoch) this.selectedBusy = false
      }
    },
    async fetchNotifications(epoch = this.accountEpoch) {
      this.notificationsSeq += 1
      const seq = this.notificationsSeq
      this.notificationsAbort?.abort()
      const abort = new AbortController()
      this.notificationsAbort = abort
      this.notificationsBusy = true
      this.notificationsError = ''
      try {
        const result = await api.listNotifications(this.mode, { limit: 50 }, { signal: abort.signal })
        if (seq !== this.notificationsSeq || epoch !== this.accountEpoch) return
        this.notifications = result.data?.items || []
        this.unreadCount = result.data?.unread_count || 0
      } catch (error) {
        if (seq !== this.notificationsSeq || epoch !== this.accountEpoch || error?.code === 'ABORTED') return
        this.notificationsError = errText(error)
      } finally {
        if (seq === this.notificationsSeq) this.notificationsBusy = false
      }
    },
    async patchNotification(id, status) {
      const epoch = this.accountEpoch
      await api.patchNotification(this.mode, id, status)
      await this.fetchNotifications(epoch)
    },
    async setMute(id, muted) {
      const epoch = this.accountEpoch
      await api.setMute(this.mode, id, muted)
      await Promise.all([this.fetchEvents(epoch), this.fetchNotifications(epoch)])
    },
    async fetchDataStatus(epoch = this.accountEpoch) {
      const result = await api.getDataStatus(this.mode)
      if (epoch === this.accountEpoch) this.dataStatus = result.data
    },
    async refreshVisible() {
      const epoch = this.accountEpoch
      await Promise.all([this.fetchEvents(epoch), this.fetchNotifications(epoch), this.fetchDataStatus(epoch)])
    },
    async fetchReviews() {
      const epoch = this.accountEpoch
      this.admin.reviewsBusy = true
      this.admin.reviewsError = ''
      try {
        const result = await api.listReviewItems(this.mode, { limit: 50 })
        if (epoch === this.accountEpoch) this.admin.reviews = result.data?.items || []
      } catch (error) {
        if (epoch === this.accountEpoch) this.admin.reviewsError = errText(error)
      } finally {
        if (epoch === this.accountEpoch) this.admin.reviewsBusy = false
      }
    },
    async resolveReview(id, action, eventId = '') {
      const epoch = this.accountEpoch
      this.admin.busy = true
      this.admin.importError = ''
      try {
        await api.resolveReviewItem(this.mode, id, { action, event_id: eventId, expected_version: 1, reason: '管理员纠错：核对来源修订与抽取结果' }, key())
        if (epoch === this.accountEpoch) await this.fetchReviews()
      } catch (error) {
        if (epoch === this.accountEpoch) this.admin.importError = errText(error)
      } finally {
        if (epoch === this.accountEpoch) this.admin.busy = false
      }
    },
    async importSource(body) {
      const epoch = this.accountEpoch
      this.admin.busy = true
      this.admin.importError = ''
      try {
        await api.importSourceRevision(this.mode, body, key())
        if (epoch === this.accountEpoch) this.admin.importError = ''
      } catch (error) {
        if (epoch === this.accountEpoch) this.admin.importError = errText(error)
      } finally {
        if (epoch === this.accountEpoch) this.admin.busy = false
      }
    },
    async createIngestion(body) {
      const epoch = this.accountEpoch
      this.admin.busy = true
      this.admin.importError = ''
      try {
        const result = await api.createIngestionJob(this.mode, body, key())
        if (epoch === this.accountEpoch) this.admin.jobs = [result.data, ...this.admin.jobs]
      } catch (error) {
        if (epoch === this.accountEpoch) this.admin.importError = errText(error)
      } finally {
        if (epoch === this.accountEpoch) this.admin.busy = false
      }
    },
    async replay(action) {
      const epoch = this.accountEpoch
      const fingerprint = action + ':' + (this.session?.replay_version || 0)
      if (!this.replay.key || this.replay.fingerprint !== fingerprint) {
        this.replay.fingerprint = fingerprint
        this.replay.key = key()
      }
      this.replay.busy = true
      this.replay.error = ''
      try {
        const body = { action, expected_version: this.session?.replay_version || 0 }
        if (action === 'reset') body.branch = 'main'
        const result = await api.replay(this.mode, body, this.replay.key)
        if (epoch !== this.accountEpoch) return
        this.replay.result = result.data
        this.session = { ...(this.session || {}), ...result.data }
        this.replay.key = ''
        this.replay.fingerprint = ''
        if (epoch === this.accountEpoch) this.replay.busy = false
        await this.refreshVisible()
      } catch (error) {
        if (epoch === this.accountEpoch) this.replay.error = errText(error)
      } finally {
        if (epoch === this.accountEpoch) this.replay.busy = false
      }
    }
  }
})
