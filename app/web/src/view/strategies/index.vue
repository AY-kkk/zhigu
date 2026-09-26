<template>
  <div class="workbench" :class="{ 'is-narrow': narrow, 'is-mobile': mobile }">
    <header class="toolbar">
      <InstrumentSearch />
      <p v-if="store.instrument" class="ident" data-testid="quote-ident">
        <strong>{{ store.instrument.name }}</strong>
        <span>{{ store.instrument.instrument_id }}</span>
        <span class="stale">{{ freshness }}</span>
      </p>
      <div class="seg" role="group" aria-label="周期">
        <button v-for="p in periods" :key="p" type="button" class="zg-btn zg-btn-ghost" :class="{ on: store.period === p }" @click="store.setPeriod(p)">{{ periodLabel[p] }}</button>
      </div>
      <div class="seg" role="group" aria-label="复权">
        <button v-for="a in adjusts" :key="a" type="button" class="zg-btn zg-btn-ghost" :class="{ on: store.adjust === a }" @click="store.setAdjust(a)">{{ adjustLabel[a] }}</button>
      </div>
      <button v-if="!mobile && !narrow" type="button" class="zg-btn zg-btn-ghost" @click="store.sidebarOpen = !store.sidebarOpen">{{ store.sidebarOpen ? '收起' : '策略' }}</button>
    </header>
    <nav v-if="mobile" class="tabs" aria-label="工作区">
      <button type="button" :class="{ on: store.mobileTab === 'quote' }" @click="store.mobileTab = 'quote'">行情</button>
      <button type="button" :class="{ on: store.mobileTab === 'strategy' }" @click="store.mobileTab = 'strategy'">策略</button>
      <button type="button" :class="{ on: store.mobileTab === 'result' }" @click="store.mobileTab = 'result'">结果</button>
    </nav>
    <div class="body">
      <section v-show="!mobile || store.mobileTab === 'quote'" class="chart-col">
        <QuoteStrip :bar="store.hover" :quote="store.quote" :values="store.hoverValues" />
        <p v-if="store.ohlcvBusy" class="state">正在加载行情</p>
        <p v-else-if="store.ohlcvError" class="state err">{{ store.ohlcvError }}</p>
        <p v-else-if="!store.instrument" class="state">搜索一只股票，查看 K 线和指标。回测是历史模拟。</p>
        <div v-else class="chart-row">
          <IndicatorPanel :bar="store.hover" :bars="store.bars" :series="store.series" />
          <ChartPane
            :bars="store.bars"
            :series="store.series"
            :indicators="store.chartIndicators"
            :fills="store.trades?.fills || []"
            :orders="store.trades?.orders || []"
            :signals="store.results?.signals || []"
            :has-more="Boolean(store.ohlcvMeta?.has_more)"
            :loading-more="store.ohlcvLoadingMore"
            :precision="pricePrecision"
            @crosshair="store.hover = $event"
            @need-older="store.loadMore()"
          />
        </div>
        <div class="ind-dock">
          <span class="dock-label">指标</span>
          <span v-for="row in store.chartIndicators" :key="row.id" class="chip">
            <button type="button" class="chip-main" @click="editId = editId === row.id ? '' : row.id">{{ chipText(row) }}</button>
            <button type="button" :aria-label="'移除 ' + row.type" @click="store.removeIndicator(row.id)">×</button>
            <span v-if="editId === row.id" class="pop">
              <label v-for="(val, key) in row.params" :key="key">{{ paramLabel[key] || key }}
                <input :value="val" type="number" @change="store.updateIndicatorParam(row.id, key, $event.target.value)">
              </label>
              <p v-if="!Object.keys(row.params || {}).length" class="hint">这个指标没有参数</p>
            </span>
          </span>
          <select aria-label="添加指标" @change="addInd($event)">
            <option value="">添加指标</option>
            <option v-for="r in store.registry" :key="r.type" :value="r.type">{{ r.type }}</option>
          </select>
          <button type="button" class="zg-btn zg-btn-ghost" @click="store.applyStrategyIndicators">显示策略指标</button>
          <button v-if="store.ohlcvMeta?.has_more" type="button" class="zg-btn zg-btn-ghost" :disabled="store.ohlcvLoadingMore" @click="store.loadMore">{{ store.ohlcvLoadingMore ? '正在加载更早行情' : '更早行情' }}</button>
        </div>
      </section>
      <StrategyRail
        v-show="(!mobile && (store.sidebarOpen || drawer)) || (mobile && store.mobileTab === 'strategy')"
        :drawer="drawer"
        :mobile="mobile"
      />
    </div>
    <section v-show="!mobile || store.mobileTab === 'result'" class="results" data-testid="backtest-results">
      <div class="result-bar">
        <h2>回测结果</h2>
        <p v-if="store.btBusy" class="hint">正在按规则计算这一只股票的买卖点</p>
        <p v-else-if="store.btError" class="err">{{ store.btError }}</p>
        <p v-else-if="!store.results" class="hint">运行后看收益。买卖点画在 K 线上。历史模拟，不是实盘。</p>
        <template v-else>
          <dl class="metrics">
            <div><dt>总收益</dt><dd>{{ ratioText(store.results.metrics?.total_return) }}</dd></div>
            <div><dt>最大回撤</dt><dd>{{ ratioText(store.results.metrics?.max_drawdown) }}</dd></div>
            <div><dt>胜率</dt><dd>{{ store.results.metrics?.win_rate == null ? '无已完成交易' : ratioText(store.results.metrics.win_rate) }}</dd></div>
            <div><dt>买入</dt><dd>{{ dealCount.buy }}</dd></div>
            <div><dt>卖出</dt><dd>{{ dealCount.sell }}</dd></div>
          </dl>
          <button type="button" class="zg-btn zg-btn-ghost" @click="detailsOpen = !detailsOpen">{{ detailsOpen ? '收起明细' : '成交与信号' }}</button>
          <button type="button" class="zg-btn zg-btn-ghost" @click="store.exportCSV">导出成交 CSV</button>
        </template>
      </div>
      <div v-if="detailsOpen && store.results" class="result-detail">
        <div class="seg" role="tablist" aria-label="回测结果">
          <button type="button" :class="{ on: store.resultTab === 'overview' }" @click="store.resultTab = 'overview'">概览</button>
          <button type="button" :class="{ on: store.resultTab === 'equity' }" @click="store.resultTab = 'equity'">净值回撤</button>
          <button type="button" :class="{ on: store.resultTab === 'trades' }" @click="store.resultTab = 'trades'">成交</button>
          <button type="button" :class="{ on: store.resultTab === 'signals' }" @click="store.resultTab = 'signals'">信号</button>
          <button type="button" :class="{ on: store.resultTab === 'notes' }" @click="store.resultTab = 'notes'">运行说明</button>
        </div>
        <template v-if="store.resultTab === 'overview'">
          <p class="hint">基准为同一股票买入持有。无风险利率 {{ store.results.metrics?.rf }}。结果 hash {{ store.results.result_hash?.slice(0, 12) }}</p>
        </template>
        <template v-else-if="store.resultTab === 'equity'">
          <table v-if="store.results.equity?.length">
            <thead><tr><th>日期</th><th>现金</th><th>市值</th><th>净值</th><th>回撤</th></tr></thead>
            <tbody>
              <tr v-for="row in equityTail" :key="row.trade_date">
                <td>{{ row.trade_date }}</td><td>{{ row.cash }}</td><td>{{ row.market_value }}</td><td>{{ row.equity }}</td><td>{{ row.drawdown }}</td>
              </tr>
            </tbody>
          </table>
          <p v-else class="hint">没有净值序列。</p>
        </template>
        <template v-else-if="store.resultTab === 'trades'">
          <table v-if="store.trades?.fills?.length">
            <thead><tr><th>日期</th><th>方向</th><th>数量</th><th>价格</th><th>现金变化</th></tr></thead>
            <tbody>
              <tr v-for="f in store.trades.fills" :key="f.id">
                <td>{{ f.fill_date }}</td>
                <td>{{ fillSide(f, store.trades.orders) === 'buy' ? '买入' : '卖出' }}</td>
                <td>{{ f.qty }}</td><td>{{ f.price }}</td><td>{{ f.cash_delta }}</td>
              </tr>
            </tbody>
          </table>
          <p v-else>没有成交。信号不等于交易。</p>
          <h3>订单与未成交原因</h3>
          <table v-if="store.trades?.orders?.length">
            <thead><tr><th>提交日</th><th>方向</th><th>状态</th><th>原因</th></tr></thead>
            <tbody>
              <tr v-for="o in store.trades.orders" :key="o.id">
                <td>{{ o.submitted_date }}</td>
                <td>{{ o.side === 'buy' ? '买入' : o.side === 'sell' ? '卖出' : o.side }}</td>
                <td>{{ o.status }}</td><td>{{ o.reason || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </template>
        <template v-else-if="store.resultTab === 'signals'">
          <table v-if="signalRows.length">
            <thead><tr><th>日期</th><th>买卖点</th></tr></thead>
            <tbody>
              <tr v-for="s in signalRows" :key="s.date + s.kind">
                <td>{{ s.date }}</td><td>{{ s.kind === 'entry' ? '买点' : s.kind === 'exit' ? '卖点' : s.kind }}</td>
              </tr>
            </tbody>
          </table>
          <p v-else class="hint">没有信号。</p>
        </template>
        <ul v-else class="plain">
          <li v-for="a in resultAssumptions" :key="a">{{ a }}</li>
        </ul>
      </div>
    </section>
  </div>
</template>
<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import InstrumentSearch from '../../components/strategies/InstrumentSearch.vue'
import QuoteStrip from '../../components/strategies/QuoteStrip.vue'
import ChartPane from '../../components/strategies/ChartPane.vue'
import IndicatorPanel from '../../components/strategies/IndicatorPanel.vue'
import StrategyRail from '../../components/strategies/StrategyRail.vue'
import { fillSide } from '../../components/strategies/tradeSide.js'
import { useStrategyWorkspace } from '../../stores/strategyWorkspace.js'
import { useStrategyMarket } from '../../stores/strategyMarket.js'

const store = useStrategyWorkspace()
const editId = ref('')
const detailsOpen = ref(false)
const width = ref(typeof window === 'undefined' ? 1440 : window.innerWidth)
const mobile = computed(() => width.value < 768)
const narrow = computed(() => width.value < 1100)
const drawer = computed(() => !mobile.value && narrow.value && store.sidebarOpen)
const periods = ['1d', '1w', '1mo']
const periodLabel = { '1d': '日', '1w': '周', '1mo': '月' }
const adjusts = ['raw', 'qfq', 'hfq']
const adjustLabel = { raw: '不复权', qfq: '前复权', hfq: '后复权' }
const paramLabel = { n: '周期', fast: '快线', slow: '慢线', signal: '信号', k: '倍数', m1: 'M1', m2: 'M2', ma5: '量均5', ma10: '量均10' }
const freshness = computed(() => {
  const last = store.quote?.last ? `最新 ${store.quote.last}` : ''
  const status = store.ohlcvMeta?.quality?.freshness_status
  const fresh = status === 'fresh' || status === 'daily_complete'
  if (fresh) return last || '日线已完成'
  return last ? `${last} · 时效未验证` : '时效未验证'
})
const pricePrecision = computed(() => store.instrument?.exchange === 'HKEX' ? 3 : 2)
const resultAssumptions = computed(() => (Array.isArray(store.results?.assumptions) ? store.results.assumptions : []))
const equityTail = computed(() => (store.results?.equity || []).slice(-12))
const signalRows = computed(() => (Array.isArray(store.results?.signals) ? store.results.signals : []))
const dealCount = computed(() => {
  const fills = store.trades?.fills || []
  const orders = store.trades?.orders || []
  return fills.reduce((acc, row) => {
    if (fillSide(row, orders) === 'buy') acc.buy += 1
    else acc.sell += 1
    return acc
  }, { buy: 0, sell: 0 })
})
function ratioText(v) {
  if (v == null || v === '') return '—'
  const n = Number(v)
  if (!Number.isFinite(n)) return String(v)
  return `${(n * 100).toFixed(2)}%`
}
function chipText(row) {
  const vals = Object.values(row.params || {})
  if (!vals.length) return row.type
  if (row.type === 'MA' || row.type === 'EMA' || row.type === 'RSI' || row.type === 'WR' || row.type === 'BIAS' || row.type === 'CCI' || row.type === 'ATR') {
    return `${row.type}${vals[0]}`
  }
  return `${row.type} ${vals.join('/')}`
}
function addInd(ev) {
  const type = ev.target.value
  ev.target.value = ''
  if (!type) return
  store.addIndicator(type)
}
function onResize() { width.value = window.innerWidth }
const route = useRoute()
onMounted(async () => {
  store.boot()
  window.__zgGenerate = () => store.generate()
  window.addEventListener('resize', onResize)
  // 市场复制深链：加载本人草稿（外人 ID 404），清除旧版本与旧回测结果。
  if (route.query.draft_id) {
    await store.loadDraft(String(route.query.draft_id))
  }
  // 市场详情「让 AI 解释」：先明确目标草稿（按市场版本复制并等待加载完成），
  // 再以 explain 模式生成说明；不解释旧草稿（R5）。
  if (route.query.explain_market) {
    const mktStore = useStrategyMarket()
    const vid = String(route.query.explain_version || '')
    if (vid) {
      const copied = await mktStore.copy(String(route.query.explain_market), { market_version_id: vid })
      if (copied?.draft_id) {
        await store.loadDraft(copied.draft_id)
        await store.explainDraft()
      }
    }
  }
})
// 未保存内容离开提示。
onBeforeRouteLeave(() => {
  if (!store.hasUnsavedEdits) return true
  return window.confirm('有未保存的规则修改，确定离开吗？离开后未保存的修改将丢失。')
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  store.stopQuote()
})
</script>
<style scoped>
.workbench {
  flex: 1; min-height: 0; display: flex; flex-direction: column; position: relative;
  background: var(--zg-paper); color: var(--zg-ink);
}
.toolbar {
  display: flex; gap: 10px; align-items: center; flex: none; flex-wrap: wrap;
  padding: 6px 12px; border-bottom: 1px solid var(--zg-line); background: var(--zg-surface);
}
.toolbar :deep(.zg-btn),
.ind-dock :deep(.zg-btn),
.result-bar :deep(.zg-btn) { min-height: 32px; padding: 0 10px; font-size: 13px; }
.ident { margin: 0; display: flex; align-items: baseline; gap: 8px; font-size: 13px; white-space: nowrap; }
.ident span, .stale { color: var(--zg-text-secondary); font-size: 12px; }
.seg { display: flex; gap: 4px; flex: none; }
.seg .on, .seg button.on { background: var(--zg-surface-muted); }
.tabs { display: flex; border-bottom: 1px solid var(--zg-line); flex: none; }
.tabs button {
  flex: 1; height: 40px; border: 0; background: transparent; font: inherit; cursor: pointer;
}
.tabs .on { box-shadow: inset 0 -2px 0 var(--zg-action); }
.body { flex: 1; min-height: 0; display: flex; }
.chart-col { flex: 1; min-width: 0; display: flex; flex-direction: column; position: relative; z-index: 1; overflow: auto; }
.chart-row { flex: 1 0 auto; min-height: 0; display: flex; align-items: stretch; }
h2, h3 { margin: 0; font-size: 15px; font-family: var(--zg-font-editorial); }
.hint { margin: 0; font-size: 12px; color: var(--zg-text-secondary); }
.err, .state.err { color: var(--zg-error-fg); }
.results {
  flex: none; border-top: 1px solid var(--zg-line); background: var(--zg-surface);
}
.result-bar {
  display: flex; align-items: center; gap: 16px; flex-wrap: wrap;
  padding: 6px 12px; min-height: 40px;
}
.result-detail { max-height: 220px; overflow: auto; padding: 0 12px 12px; border-top: 1px solid var(--zg-line); }
.metrics { display: flex; gap: 16px; margin: 0; }
dt { font-size: 12px; color: var(--zg-text-secondary); }
dd { margin: 0; font-family: var(--zg-font-number); }
table { width: 100%; border-collapse: collapse; font-size: 12px; font-family: var(--zg-font-number); }
th, td { text-align: left; padding: 4px 8px; border-bottom: 1px solid var(--zg-line); }
.ind-dock {
  display: flex; gap: 8px; align-items: center; flex: none;
  padding: 6px 12px; border-top: 1px solid var(--zg-line); font-size: 12px;
  overflow-x: auto;
}
.dock-label { color: var(--zg-text-secondary); flex: none; }
.chip {
  position: relative; flex: none; display: inline-flex; align-items: center; gap: 4px;
  border: 1px solid var(--zg-line); background: var(--zg-surface-muted); border-radius: 999px; padding: 2px 8px;
}
.chip-main, .chip button {
  border: 0; background: transparent; cursor: pointer; font: inherit; font-size: 12px; padding: 0;
}
.pop {
  position: absolute; bottom: calc(100% + 6px); left: 0; z-index: 6;
  display: grid; gap: 6px; min-width: 148px; padding: 8px;
  background: var(--zg-surface); border: 1px solid var(--zg-line); border-radius: 8px;
  box-shadow: var(--zg-shadow-overlay);
}
.pop input { width: 72px; border: 1px solid var(--zg-control-border); border-radius: 6px; padding: 4px; font: inherit; }
.ind-dock select {
  flex: none; width: auto; border: 1px solid var(--zg-control-border); border-radius: 6px; padding: 4px 8px; font: inherit; background: #fff;
}
.state { margin: 24px; color: var(--zg-text-secondary); }
.plain { margin: 8px 0; padding-left: 18px; font-size: 12px; }
.is-mobile .results { max-height: none; flex: 1; overflow: auto; }
.is-mobile .result-detail { max-height: none; }
</style>
