import { defineStore } from 'pinia'
import * as api from '../api/strategies.js'

function newKey() {
  return crypto.randomUUID()
}

function errText(e) {
  return e?.message || '请求失败'
}

function payloadOf(res) {
  if (!res || typeof res !== 'object') return {}
  let d = res.data
  if (d && typeof d === 'object' && d.data && typeof d.data === 'object' && (d.error === null || d.error === undefined)) {
    d = d.data
  }
  if (d && typeof d === 'object' && ('generation_id' in d || 'dsl' in d || 'run_id' in d || 'draft_id' in d || 'strategy_id' in d || typeof d.status === 'string')) {
    return d
  }
  if (res.generation_id || res.dsl || res.run_id || res.draft_id) return res
  if (d?.data && typeof d.data === 'object') return d.data
  return d || {}
}

function walkSetK(node, value) {
  if (!node || typeof node !== 'object') return
  if (Array.isArray(node)) {
    node.forEach((row) => walkSetK(row, value))
    return
  }
  if (node.left === 'kdj.k' && node.right && typeof node.right === 'object') {
    node.right.constant = String(value)
  }
  if (node.all) walkSetK(node.all, value)
  if (node.any) walkSetK(node.any, value)
}

function readK(node) {
  if (!node || typeof node !== 'object') return ''
  if (Array.isArray(node)) {
    for (const row of node) {
      const v = readK(row)
      if (v) return v
    }
    return ''
  }
  if (node.left === 'kdj.k' && node.right?.constant != null) return String(node.right.constant)
  return readK(node.all) || readK(node.any)
}

export const useStrategyWorkspace = defineStore('strategyWorkspace', {
  state: () => ({
    query: '',
    hits: [],
    searchBusy: false,
    searchError: '',
    instrument: null,
    coverage: null,
    bars: [],
    ohlcvMeta: null,
    ohlcvBusy: false,
    ohlcvError: '',
    seq: 0,
    period: '1d',
    adjust: 'raw',
    hover: null,
    chartIndicators: [
      { id: 'ma20', type: 'MA', params: { n: 20 } },
      { id: 'vol', type: 'VOL', params: { ma5: 5, ma10: 10 } }
    ],
    series: [],
    registry: [],
    prompt: '',
    genStatus: '',
    genError: '',
    generationId: '',
    draft: null,
    draftRev: 0,
    dslText: '',
    showAdvanced: false,
    sidebarOpen: true,
    mobileTab: 'quote',
    strategyId: '',
    versionId: '',
    strategies: [],
    runs: [],
    backtest: null,
    results: null,
    trades: null,
    resultTab: 'overview',
    btBusy: false,
    btError: '',
    saveBusy: false,
    saveError: '',
    saveNotice: '',
    initialCash: '100000',
    slippage: '10',
    start: '',
    end: '',
    catalogMeta: null,
    workspaceRevision: 0,
    watchlist: [],
    persistTimer: 0
  }),
  getters: {
    hoverValues(state) {
      if (!state.hover || !state.bars.length) return {}
      const i = state.bars.findIndex((b) => b.time === state.hover.time)
      if (i < 0) return {}
      const out = {}
      state.series.forEach((row) => {
        Object.entries(row.fields || {}).forEach(([name, pts]) => {
          if (pts && pts[i] != null) out[`${row.type}.${name}`] = pts[i]
        })
      })
      return out
    },
    kdjK(state) {
      return readK(state.draft?.dsl?.entry)
    }
  },
  actions: {
    async boot() {
      await Promise.all([this.loadRegistry(), this.loadWorkspace(), this.refreshStrategies()])
    },
    async search() {
      this.searchBusy = true
      this.searchError = ''
      try {
        const res = await api.searchInstruments({ q: this.query, limit: 20 })
        this.hits = res.data.items || []
        this.catalogMeta = {
          catalog_version: res.data.catalog_version,
          catalog_as_of: res.data.catalog_as_of,
          freshness_status: res.data.freshness_status
        }
      } catch (e) {
        this.searchError = errText(e)
        this.hits = []
      } finally {
        this.searchBusy = false
      }
    },
    async loadRegistry() {
      try {
        const res = await api.listIndicators()
        this.registry = res.data.items || []
      } catch {
        this.registry = []
      }
    },
    async loadWorkspace() {
      try {
        const res = await api.getWorkspace()
        this.workspaceRevision = res.data.revision || 0
        this.watchlist = Array.isArray(res.data.watchlist) ? res.data.watchlist : []
        if (Array.isArray(res.data.chart_indicators) && res.data.chart_indicators.length) {
          this.chartIndicators = res.data.chart_indicators
        }
        if (res.data.last_instrument_id) {
          await this.selectInstrument(res.data.last_instrument_id)
        }
      } catch {
        /* first visit has empty workspace */
      }
    },
    schedulePersist() {
      clearTimeout(this.persistTimer)
      this.persistTimer = setTimeout(() => this.persistWorkspace(), 400)
    },
    async persistWorkspace() {
      try {
        const last = this.instrument?.instrument_id || null
        const res = await api.putWorkspace({
          revision: this.workspaceRevision,
          watchlist: this.watchlist,
          layout: { period: this.period, adjust: this.adjust },
          chart_indicators: this.chartIndicators,
          last_instrument_id: last
        })
        this.workspaceRevision = res.data.revision || this.workspaceRevision
      } catch (e) {
        if (e.code === 'REVISION_CONFLICT') {
          this.loadWorkspace()
        }
      }
    },
    async selectInstrument(id) {
      this.seq += 1
      const seq = this.seq
      this.ohlcvBusy = true
      this.ohlcvError = ''
      this.bars = []
      this.instrument = null
      try {
        const [detail, ohlcv] = await Promise.all([
          api.getInstrument(id),
          api.getOHLCV({ instrument_id: id, period: this.period, adjust: this.adjust, limit: 400 })
        ])
        if (seq !== this.seq) return
        this.instrument = detail.data.instrument
        this.coverage = detail.data.coverage
        this.bars = ohlcv.data.bars || []
        this.ohlcvMeta = ohlcv.data
        if (this.bars.length) {
          const last = this.bars[this.bars.length - 1]
          this.hover = last
          if (!this.end) this.end = last.time
          if (!this.start && this.bars.length > 1) this.start = this.bars[Math.max(0, this.bars.length - 250)].time
        }
        if (this.instrument?.instrument_id && !this.watchlist.includes(this.instrument.instrument_id)) {
          this.watchlist = [this.instrument.instrument_id, ...this.watchlist].slice(0, 12)
        }
        this.schedulePersist()
        await this.refreshIndicators(seq)
      } catch (e) {
        if (seq !== this.seq) return
        this.ohlcvError = errText(e)
      } finally {
        if (seq === this.seq) this.ohlcvBusy = false
      }
    },
    async loadMore() {
      const cursor = this.ohlcvMeta?.next_cursor
      if (!cursor || !this.instrument?.instrument_id) return
      const seq = this.seq
      try {
        const ohlcv = await api.getOHLCV({
          instrument_id: this.instrument.instrument_id,
          period: this.period,
          adjust: this.adjust,
          cursor,
          limit: 400
        })
        if (seq !== this.seq) return
        const older = ohlcv.data.bars || []
        const seen = new Set(older.map((b) => b.time))
        this.bars = [...older, ...this.bars.filter((b) => !seen.has(b.time))]
        this.ohlcvMeta = { ...ohlcv.data, bars: this.bars }
        await this.refreshIndicators(seq)
      } catch (e) {
        this.ohlcvError = errText(e)
      }
    },
    async setPeriod(period) {
      this.period = period
      this.schedulePersist()
      if (this.instrument?.instrument_id) await this.selectInstrument(this.instrument.instrument_id)
    },
    async setAdjust(adjust) {
      this.adjust = adjust
      this.schedulePersist()
      if (this.instrument?.instrument_id) await this.selectInstrument(this.instrument.instrument_id)
    },
    async refreshIndicators(seq) {
      if (!this.instrument?.instrument_id || !this.chartIndicators.length) {
        this.series = []
        return
      }
      try {
        const res = await api.indicatorSeries({
          instrument_id: this.instrument.instrument_id,
          period: this.period,
          adjust: this.adjust,
          indicators: this.chartIndicators
        })
        if (seq && seq !== this.seq) return
        this.series = res.data.series || []
      } catch {
        if (seq && seq !== this.seq) return
        this.series = []
      }
    },
    addIndicator(type) {
      const d = this.registry.find((r) => r.type === type)
      this.chartIndicators = [
        ...this.chartIndicators,
        { id: `${type.toLowerCase()}${Date.now()}`, type, params: { ...(d?.default_params || {}) } }
      ]
      this.schedulePersist()
      this.refreshIndicators(this.seq)
    },
    removeIndicator(id) {
      this.chartIndicators = this.chartIndicators.filter((r) => r.id !== id)
      this.schedulePersist()
      this.refreshIndicators(this.seq)
    },
    updateIndicatorParam(id, key, value) {
      const n = Number(value)
      this.chartIndicators = this.chartIndicators.map((row) => {
        if (row.id !== id) return row
        return { ...row, params: { ...row.params, [key]: Number.isFinite(n) ? n : row.params[key] } }
      })
      this.schedulePersist()
      this.refreshIndicators(this.seq)
    },
    applyStrategyIndicators() {
      const inds = this.draft?.dsl?.indicators
      if (!Array.isArray(inds)) return
      this.chartIndicators = inds.map((row) => ({ ...row }))
      this.schedulePersist()
      this.refreshIndicators(this.seq)
    },
    async generate() {
      if (!this.instrument?.instrument_id) {
        this.genStatus = 'failed'
        this.genError = '请先选择股票'
        return { status: 'failed', hasDsl: Boolean(this.draft?.dsl), err: this.genError }
      }
      this.genStatus = 'queued'
      this.genError = ''
      try {
        const res = await api.generateDraft(
          { text: this.prompt, instrument_id: this.instrument.instrument_id, draft_id: this.draft?.draft_id, base_revision: this.draft?.revision },
          newKey()
        )
        const started = payloadOf(res)
        const id = started.generation_id
        this.generationId = id
        if (!id) {
          this.genStatus = 'failed'
          this.genError = '生成任务没有返回 ID'
          return { status: 'failed', hasDsl: false, err: this.genError }
        }
        for (let i = 0; i < 40; i += 1) {
          const g = payloadOf(await api.getGeneration(id))
          const dsl = g.dsl && typeof g.dsl === 'object' ? g.dsl : null
          this.genStatus = g.status || this.genStatus
          this.draft = {
            draft_id: g.draft_id || started.draft_id,
            revision: g.draft_revision ?? g.revision,
            dsl,
            assumptions: Array.isArray(g.assumptions) ? g.assumptions : [],
            clarification: Array.isArray(g.clarification) ? g.clarification : [],
            compiled: g.compiled
          }
          this.draftRev += 1
          if (dsl) this.dslText = JSON.stringify(dsl, null, 2)
          if (g.error_message) this.genError = g.error_message
          if (['ready', 'failed', 'unsupported', 'needs_clarification', 'canceled'].includes(g.status)) break
          await new Promise((r) => setTimeout(r, 250))
        }
        if (!['ready', 'failed', 'unsupported', 'needs_clarification', 'canceled'].includes(this.genStatus)) {
          this.genStatus = 'failed'
          this.genError = this.genError || '生成超时'
        }
      } catch (e) {
        this.genStatus = 'failed'
        this.genError = errText(e)
      }
      return { status: this.genStatus, hasDsl: Boolean(this.draft?.dsl), err: this.genError, id: this.generationId }
    },
    async cancelGenerate() {
      if (!this.generationId) return
      try {
        await api.cancelGeneration(this.generationId)
        this.genStatus = 'canceled'
      } catch (e) {
        this.genError = errText(e)
      }
    },
    setKdjK(value) {
      if (!this.draft?.dsl) return
      const dsl = JSON.parse(JSON.stringify(this.draft.dsl))
      walkSetK(dsl.entry, value)
      this.draft = { ...this.draft, dsl }
      this.dslText = JSON.stringify(dsl, null, 2)
      this.draftRev += 1
    },
    async applyDraft() {
      if (!this.draft?.draft_id) {
        this.genError = '请先生成规则'
        return
      }
      try {
        const dsl = this.showAdvanced && this.dslText ? JSON.parse(this.dslText) : this.draft.dsl
        const res = await api.patchDraft(this.draft.draft_id, { revision: this.draft.revision, dsl })
        this.draft = { ...this.draft, ...res.data }
        if (res.data.dsl) this.dslText = JSON.stringify(res.data.dsl, null, 2)
        this.draftRev += 1
        this.saveNotice = `规则已更新，修订 ${res.data.revision}`
      } catch (e) {
        this.genError = errText(e)
      }
    },
    async saveCurrent() {
      if (!this.draft?.draft_id) {
        this.saveError = '请先生成可执行规则'
        return null
      }
      this.saveBusy = true
      this.saveError = ''
      try {
        const payload = { draft_id: this.draft.draft_id, revision: this.draft.revision, base_version_id: this.versionId }
        const saved = this.strategyId
          ? await api.saveStrategyVersion(this.strategyId, payload, newKey())
          : await api.saveStrategy(payload, newKey())
        this.strategyId = saved.data.strategy_id
        this.versionId = saved.data.version_id
        this.saveNotice = `已保存版本 ${saved.data.revision}`
        await this.refreshStrategies()
        return saved.data
      } catch (e) {
        this.saveError = errText(e)
        return null
      } finally {
        this.saveBusy = false
      }
    },
    async refreshStrategies() {
      try {
        const res = await api.listStrategies({ limit: 20 })
        this.strategies = res.data.items || []
      } catch {
        this.strategies = []
      }
    },
    async openStrategy(id) {
      if (!id) return
      try {
        const res = await api.getStrategy(id)
        const current = (res.data.versions || [])[0]
        this.strategyId = id
        this.versionId = current?.id || res.data.strategy?.current_version_id || ''
        if (current?.dsl) {
          const imported = await api.importDraft({
            dsl: current.dsl,
            instrument_id: current.dsl.instrument_id,
            text: current.name || ''
          })
          this.draft = imported.data
          this.dslText = JSON.stringify(imported.data.dsl, null, 2)
        }
        this.runs = res.data.recent_runs || []
        if (this.draft?.dsl?.instrument_id) await this.selectInstrument(this.draft.dsl.instrument_id)
      } catch (e) {
        this.saveError = errText(e)
      }
    },
    async runBacktest() {
      if (!this.draft?.dsl) {
        this.btError = '请先生成可执行规则'
        return
      }
      this.btBusy = true
      this.btError = ''
      this.results = null
      this.trades = null
      try {
        const saved = await this.saveCurrent()
        if (!saved) {
          this.btError = this.saveError || '保存失败，无法回测'
          return
        }
        const payload = {
          strategy_version_id: saved.version_id,
          instrument_id: this.instrument.instrument_id,
          start: this.start,
          end: this.end,
          initial_cash: this.initialCash,
          currency: this.instrument.currency,
          fee_schedule_id: 'product_default_assumption',
          commission_config: '0.00025',
          slippage_bps: this.slippage,
          participation_cap: '0.01',
          benchmark: 'buy_hold',
          execution_profile_id: 'next_session_open'
        }
        const created = await api.createBacktest(payload, newKey())
        const id = created.data.run_id
        let run
        for (let i = 0; i < 80; i += 1) {
          await new Promise((r) => setTimeout(r, 250))
          run = (await api.getBacktest(id)).data
          this.backtest = run
          if (['succeeded', 'failed', 'canceled', 'insufficient_data'].includes(run.status)) break
        }
        if (run?.status === 'succeeded') {
          this.results = (await api.getBacktestResults(id)).data
          this.trades = (await api.getBacktestTrades(id)).data
          this.resultTab = 'overview'
          this.mobileTab = this.mobileTab === 'quote' ? 'quote' : 'result'
        } else {
          this.btError = run?.error_message || '回测未成功'
        }
      } catch (e) {
        this.btError = errText(e)
      } finally {
        this.btBusy = false
      }
    },
    async cancelBacktest() {
      const id = this.backtest?.run_id
      if (!id) return
      try {
        await api.cancelBacktest(id)
      } catch (e) {
        this.btError = errText(e)
      }
    },
    async exportCSV() {
      const id = this.backtest?.run_id
      if (!id) return
      try {
        const res = await api.exportBacktest(id)
        const blob = res.data instanceof Blob ? res.data : new Blob([res.data], { type: 'text/csv' })
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `${id}.csv`
        a.click()
        URL.revokeObjectURL(url)
      } catch (e) {
        this.btError = errText(e)
      }
    }
  }
})
