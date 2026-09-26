import { defineStore } from 'pinia'
import * as api from '../api/strategyMarket.js'

function errText(e) {
  return e?.message || '请求失败'
}

function newKey() {
  return crypto.randomUUID()
}

const emptyFilters = () => ({ q: '', category: '', market: '', period: '', validation: '', evidence: '' })

export const useStrategyMarket = defineStore('strategyMarket', {
  state: () => ({
    account: '',
    filters: emptyFilters(),
    items: [],
    nextCursor: null,
    listBusy: false,
    listError: '',
    listSeq: 0,
    listAbort: null,
    debounceTimer: 0,
    savedScroll: 0,
    detail: null,
    detailBusy: false,
    detailError: '',
    detailSeq: 0,
    detailNotice: '',
    evidenceSection: 'overview',
    evidenceItems: [],
    evidenceNext: null,
    evidenceBusy: false,
    evidenceError: '',
    evidenceSeq: 0,
    copyBusy: false,
    copyError: '',
    copyResult: null,
    copyKey: '',
    copyFingerprint: ''
  }),
  actions: {
    // 切到其他账号：清空两个 store 相关缓存、复制 key 与列表。
    ensureAccount() {
      const user = localStorage.getItem('zhigu_user') || ''
      if (this.account && this.account !== user) {
        this.$reset()
      }
      this.account = user
    },
    syncQuery(routeQuery) {
      const f = emptyFilters()
      Object.keys(f).forEach((k) => {
        if (typeof routeQuery[k] === 'string') f[k] = routeQuery[k]
      })
      this.filters = f
    },
    queryOf() {
      const out = {}
      Object.entries(this.filters).forEach(([k, v]) => {
        if (v) out[k] = v
      })
      return out
    },
    async fetchList({ append = false } = {}) {
      if (!append) {
        this.listSeq += 1
        this.listAbort?.abort()
      }
      const seq = this.listSeq
      const ctrl = new AbortController()
      this.listAbort = ctrl
      this.listBusy = true
      this.listError = ''
      try {
        const res = await api.listMarketItems(
          {
            ...this.queryOf(),
            cursor: append ? this.nextCursor : undefined,
            limit: 20
          },
          { signal: ctrl.signal }
        )
        if (seq !== this.listSeq) return
        const items = res.data.items || []
        this.items = append ? [...this.items, ...items] : items
        this.nextCursor = res.data.next_cursor || null
      } catch (e) {
        if (seq !== this.listSeq || e?.code === 'ERR_CANCELED' || e?.name === 'CanceledError') return
        this.listError = errText(e)
        if (!append) this.items = []
      } finally {
        if (seq === this.listSeq) this.listBusy = false
      }
    },
    // 搜索防抖 300ms；筛选切换清空旧游标并从第一页重查。
    scheduleSearch() {
      clearTimeout(this.debounceTimer)
      this.debounceTimer = setTimeout(() => {
        this.nextCursor = null
        this.fetchList()
      }, 300)
    },
    setFilter(key, value) {
      this.filters = { ...this.filters, [key]: value }
      this.nextCursor = null
      this.fetchList()
    },
    resetFilters() {
      this.filters = emptyFilters()
      this.nextCursor = null
      this.fetchList()
    },
    async fetchDetail(id) {
      this.detailSeq += 1
      const seq = this.detailSeq
      this.detailBusy = true
      this.detailError = ''
      this.detailNotice = ''
      try {
        const res = await api.getMarketItem(id)
        if (seq !== this.detailSeq) return
        this.detail = res.data
        this.evidenceItems = []
        this.evidenceNext = null
        this.evidenceSection = 'overview'
      } catch (e) {
        if (seq !== this.detailSeq) return
        this.detail = null
        this.detailError = e?.code === 'MARKET_ITEM_WITHDRAWN' ? '该策略已下架，停止新复制。已有副本不受影响。' : errText(e)
      } finally {
        if (seq === this.detailSeq) this.detailBusy = false
      }
    },
    async loadEvidence(evidenceId, section, { append = false } = {}) {
      if (!evidenceId) return
      this.evidenceSeq += 1
      const seq = this.evidenceSeq
      this.evidenceSection = section
      this.evidenceBusy = true
      this.evidenceError = ''
      try {
        const res = await api.getMarketEvidence(this.detail?.id, evidenceId, {
          section,
          cursor: append ? this.evidenceNext : undefined,
          limit: 50
        })
        if (seq !== this.evidenceSeq) return
        const items = res.data.items
        this.evidenceItems = append
          ? [...this.evidenceItems, ...(items || [])]
          : items || (section === 'overview' ? [res.data] : [])
        this.evidenceNext = res.data.next_cursor || null
      } catch (e) {
        if (seq !== this.evidenceSeq) return
        this.evidenceError = errText(e)
      } finally {
        if (seq === this.evidenceSeq) this.evidenceBusy = false
      }
    },
    // 复制：同请求体复用同一幂等 key，超时可原 key 重试；只有确定结果才清 key。
    async copy(itemId, payload) {
      const fingerprint = JSON.stringify({ itemId, payload })
      if (fingerprint !== this.copyFingerprint || !this.copyKey) {
        this.copyFingerprint = fingerprint
        this.copyKey = newKey()
      }
      this.copyBusy = true
      this.copyError = ''
      try {
        const res = await api.copyMarketItem(itemId, payload, this.copyKey)
        this.copyResult = res.data
        this.copyKey = ''
        this.copyFingerprint = ''
        return res.data
      } catch (e) {
        if (e?.code === 'MARKET_VERSION_CHANGED') {
          this.copyError = '市场版本已更新，请核对最新版本后重新复制。'
          this.copyKey = ''
          this.copyFingerprint = ''
          await this.fetchDetail(itemId)
          return null
        }
        if (e?.code === 'MARKET_ITEM_WITHDRAWN' || e?.code === 'COPY_TARGET_GONE') {
          this.copyError = e.message
          this.copyKey = ''
          this.copyFingerprint = ''
          return null
        }
        // 业务性失败视为确定结果；网络/超时保留 key 供重试。
        if (e?.code && e.code !== 'NETWORK' && e.code !== 'TIMEOUT') {
          this.copyKey = ''
          this.copyFingerprint = ''
        }
        this.copyError = errText(e)
        return null
      } finally {
        this.copyBusy = false
      }
    },
    saveScroll(y) {
      this.savedScroll = y
    }
  }
})
