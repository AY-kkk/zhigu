import { defineStore } from 'pinia'
import * as api from './api.js'
import {
  normalizeCapabilities,
  normalizeHypothesis,
  normalizeNotification,
  normalizeProduct,
  normalizeRun,
  normalizeSource,
  normalizeSourceProbe,
} from './model.js'

function isCancellation(error) {
  return error?.name === 'CanceledError' || error?.code === 'ERR_CANCELED'
}

export const useFuturesStore = defineStore('futures', {
  state: () => ({
    account: '',
    generation: 0,
    capabilities: null,
    products: [],
    watchlist: null,
    runs: [],
    hypotheses: [],
    notifications: [],
    sources: [],
    sourceProbes: [],
    resources: {},
    loading: false,
    error: null,
    controllers: new Set(),
    mutationKeys: {},
    pollers: {},
  }),
  actions: {
    ensureAccount(account = '') {
      if (account !== this.account) this.resetForAccount(account)
    },
    resetForAccount(account = '') {
      for (const controller of this.controllers) controller.abort()
      for (const timer of Object.values(this.pollers)) clearTimeout(timer)
      this.controllers.clear()
      this.pollers = {}
      this.mutationKeys = {}
      this.account = account
      this.generation += 1
      this.capabilities = null
      this.products = []
      this.watchlist = null
      this.runs = []
      this.hypotheses = []
      this.notifications = []
      this.sources = []
      this.sourceProbes = []
      this.resources = {}
      this.error = null
    },
    mutationKey(operation, retry = false) {
      if (retry || !this.mutationKeys[operation]) this.mutationKeys[operation] = crypto.randomUUID()
      return this.mutationKeys[operation]
    },
    settleMutation(operation) {
      delete this.mutationKeys[operation]
    },
    async request(loader, assign, options = {}) {
      const generation = this.generation
      const controller = new AbortController()
      this.controllers.add(controller)
      this.loading = true
      if (!options.silent) this.error = null
      try {
        const value = await loader(controller.signal)
        if (generation !== this.generation) return null
        assign(value)
        return value
      } catch (error) {
        if (generation === this.generation && !isCancellation(error)) this.error = error
        throw error
      } finally {
        this.controllers.delete(controller)
        if (generation === this.generation) this.loading = false
      }
    },
    loadCapabilities() {
      return this.request(
        signal => api.getCapabilities(signal),
        value => { this.capabilities = normalizeCapabilities(value) },
      )
    },
    loadProducts() {
      return this.request(
        signal => api.getProducts(undefined, 100, signal),
        value => { this.products = (value.items || []).map(normalizeProduct) },
      )
    },
    loadWatchlist() {
      return this.request(signal => api.getWatchlist(signal), value => { this.watchlist = value })
    },
    loadRuns() {
      return this.request(
        signal => api.getRuns(undefined, 100, signal),
        value => {
          this.resources.runs = (value.items || []).map(normalizeRun)
          this.runs = value.items || []
        },
      )
    },
    loadHypotheses() {
      return this.request(
        signal => api.getHypotheses(undefined, 100, signal),
        value => {
          this.resources.hypotheses = (value.items || []).map(normalizeHypothesis)
          this.hypotheses = value.items || []
        },
      )
    },
    loadNotifications(filter = {}) {
      return this.request(
        signal => api.getNotifications(filter, 100, signal),
        value => {
          this.resources.notifications = (value.items || []).map(normalizeNotification)
          this.notifications = value.items || []
        },
      )
    },
    async batchReadNotifications(ids) {
      const unique = [...new Set(ids)]
      if (unique.length === 0 || unique.length > 100) throw new Error('INVALID_NOTIFICATION_BATCH')
      const snapshot = new Map(this.notifications.map(item => [item.id, item.read_at]))
      const now = new Date().toISOString()
      this.notifications = this.notifications.map(item => unique.includes(item.id) ? { ...item, read_at: now } : item)
      try {
        const result = await api.batchReadNotifications(unique, this.mutationKey('notifications.batch-read'))
        this.settleMutation('notifications.batch-read')
        await this.loadNotifications({}).catch(() => {})
        return result
      } catch (error) {
        this.notifications = this.notifications.map(item => snapshot.has(item.id) ? { ...item, read_at: snapshot.get(item.id) } : item)
        this.error = error
        throw error
      }
    },
    async startResearch(body, retry = false, keyOverride = '') {
      const key = keyOverride || this.mutationKey('research.create', retry)
      try {
        const run = await api.createRun(body, key)
        this.settleMutation('research.create')
        return run
      } catch (error) {
        if (isCancellation(error)) throw error
        this.error = error
        throw error
      }
    },
    async loadRun(id) {
      return this.request(
        signal => api.getRun(id, signal),
        value => { this.resources[`run:${id}`] = normalizeRun(value) },
        { silent: true },
      )
    },
    pollRun(id, interval = 3000) {
      clearTimeout(this.pollers[`run:${id}`])
      const tick = async () => {
        const value = await this.loadRun(id).catch(() => null)
        const terminal = value && ['succeeded', 'failed', 'canceled'].includes(value.status)
        if (!terminal && this.generation >= 0) this.pollers[`run:${id}`] = setTimeout(tick, interval)
      }
      this.pollers[`run:${id}`] = setTimeout(tick, interval)
    },
    stopPollingRun(id) {
      clearTimeout(this.pollers[`run:${id}`])
      delete this.pollers[`run:${id}`]
    },
    async cancelRun(id, retry = false) {
      const key = this.mutationKey(`run.cancel:${id}`, retry)
      try {
        const value = await api.cancelRun(id, key)
        this.settleMutation(`run.cancel:${id}`)
        return value
      } finally {
        this.settleMutation(`run.cancel:${id}`)
      }
    },
    loadSources() {
      return this.request(
        signal => api.getSources(undefined, 100, signal),
        value => {
          this.sources = (value.items || []).map(normalizeSource)
          this.resources.sources = this.sources
        },
      )
    },
    async runSourceProbe(body, retry = false) {
      const key = this.mutationKey('source.probe', retry)
      try {
        const value = await api.createSourceProbe(body, key)
        this.sourceProbes = [normalizeSourceProbe(value), ...this.sourceProbes]
        this.settleMutation('source.probe')
        return value
      } finally {
        this.settleMutation('source.probe')
      }
    },
    async activateLive(body, retry = false) {
      this.mutationKey('operations.live', retry)
      try {
        const value = await api.patchOperations(body)
        this.settleMutation('operations.live')
        await this.loadCapabilities().catch(() => {})
        return value
      } finally {
        this.settleMutation('operations.live')
      }
    },
  },
})
