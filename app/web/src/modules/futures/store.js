import { defineStore } from 'pinia'
import * as api from './api.js'

export const useFuturesStore = defineStore('futures', {
  state: () => ({
    account: localStorage.getItem('zhigu_user') || '',
    generation: 0,
    capabilities: null,
    products: [],
    watchlist: null,
    runs: [],
    hypotheses: [],
    notifications: [],
    loading: false,
    error: null,
    controllers: new Set()
  }),
  actions: {
    ensureAccount() {
      const account = localStorage.getItem('zhigu_user') || ''
      if (account !== this.account) this.resetAccount(account)
    },
    resetAccount(account = '') {
      for (const controller of this.controllers) controller.abort()
      this.controllers.clear()
      this.account = account
      this.generation += 1
      this.capabilities = null
      this.products = []
      this.watchlist = null
      this.runs = []
      this.hypotheses = []
      this.notifications = []
      this.error = null
    },
    async request(loader, assign) {
      this.ensureAccount()
      const generation = this.generation
      const controller = new AbortController()
      this.controllers.add(controller)
      this.loading = true
      this.error = null
      try {
        const value = await loader(controller.signal)
        if (generation !== this.generation) return null
        assign(value)
        return value
      } catch (error) {
        if (generation === this.generation && error.name !== 'CanceledError' && error.code !== 'ERR_CANCELED') this.error = error
        throw error
      } finally {
        this.controllers.delete(controller)
        if (generation === this.generation) this.loading = false
      }
    },
    loadCapabilities() {
      return this.request(signal => api.getCapabilities(signal), value => { this.capabilities = value })
    },
    loadProducts() {
      return this.request(signal => api.getProducts(undefined, 100, signal), value => { this.products = value.items || [] })
    },
    loadWatchlist() {
      return this.request(signal => api.getWatchlist(signal), value => { this.watchlist = value })
    },
    loadRuns() {
      return this.request(signal => api.getRuns(undefined, 100, signal), value => { this.runs = value.items || [] })
    },
    loadHypotheses() {
      return this.request(signal => api.getHypotheses(undefined, 100, signal), value => { this.hypotheses = value.items || [] })
    },
    loadNotifications() {
      return this.request(signal => api.getNotifications(undefined, 100, signal), value => { this.notifications = value.items || [] })
    }
  }
})
