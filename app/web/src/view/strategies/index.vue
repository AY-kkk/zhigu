<template>
  <div class="workbench" :class="{ 'is-narrow': narrow, 'is-mobile': mobile }">
    <header class="toolbar">
      <InstrumentSearch />
      <p v-if="store.instrument" class="ident" data-testid="quote-ident">
        <strong>{{ store.instrument.name }}</strong>
        <span>{{ store.instrument.instrument_id }} · {{ store.instrument.exchange }} · {{ store.instrument.currency }}</span>
        <span class="stale">{{ freshness }}</span>
      </p>
      <div class="seg" role="group" aria-label="周期">
        <button v-for="p in periods" :key="p" type="button" class="zg-btn zg-btn-ghost" :class="{ on: store.period === p }" @click="store.setPeriod(p)">{{ periodLabel[p] }}</button>
      </div>
      <div class="seg" role="group" aria-label="复权">
        <button v-for="a in adjusts" :key="a" type="button" class="zg-btn zg-btn-ghost" :class="{ on: store.adjust === a }" @click="store.setAdjust(a)">{{ a }}</button>
      </div>
      <button v-if="!mobile && !narrow" type="button" class="zg-btn zg-btn-ghost" @click="store.sidebarOpen = !store.sidebarOpen">{{ store.sidebarOpen ? '收起策略栏' : '打开策略栏' }}</button>
    </header>
    <nav v-if="mobile" class="tabs" aria-label="工作区">
      <button type="button" :class="{ on: store.mobileTab === 'quote' }" @click="store.mobileTab = 'quote'">行情</button>
      <button type="button" :class="{ on: store.mobileTab === 'strategy' }" @click="store.mobileTab = 'strategy'">策略</button>
      <button type="button" :class="{ on: store.mobileTab === 'result' }" @click="store.mobileTab = 'result'">结果</button>
    </nav>
    <div class="body">
      <section v-show="!mobile || store.mobileTab === 'quote'" class="chart-col">
        <QuoteStrip :bar="store.hover" :values="store.hoverValues" />
        <p v-if="store.ohlcvBusy" class="state">正在加载行情</p>
        <p v-else-if="store.ohlcvError" class="state err">{{ store.ohlcvError }}</p>
        <p v-else-if="!store.instrument" class="state">搜索 A 股或港股，查看日 K、成交量与指标。回测是历史模拟，不是实盘。</p>
        <ChartPane
          v-else
          :bars="store.bars"
          :series="store.series"
          :fills="store.trades?.fills || []"
          :signals="store.results?.signals || []"
          @crosshair="store.hover = $event"
        />
        <div class="ind-dock">
          <span>图表指标</span>
          <span v-for="row in store.chartIndicators" :key="row.id" class="chip">
            {{ row.type }}
            <template v-for="(val, key) in row.params" :key="key">
              <label>{{ key }}
                <input :value="val" type="number" @change="store.updateIndicatorParam(row.id, key, $event.target.value)">
              </label>
            </template>
            <button type="button" :aria-label="'移除 ' + row.type" @click="store.removeIndicator(row.id)">×</button>
          </span>
          <select aria-label="添加指标" @change="addInd($event)">
            <option value="">添加指标</option>
            <option v-for="r in store.registry" :key="r.type" :value="r.type">{{ r.type }}</option>
          </select>
          <button type="button" class="zg-btn zg-btn-ghost" @click="store.applyStrategyIndicators">将策略指标显示到图表</button>
          <button v-if="store.ohlcvMeta?.has_more" type="button" class="zg-btn zg-btn-ghost" @click="store.loadMore">更早行情</button>
        </div>
      </section>
      <aside v-show="(!mobile && (store.sidebarOpen || drawer)) || (mobile && store.mobileTab === 'strategy')" class="side" :class="{ drawer }">
        <h2>策略助手</h2>
        <p class="hint">用一句话描述规则。模型只生成可编辑 DSL，信号和收益由确定性引擎计算。管理后台启用模型配置后走独立策略生成；否则使用启发式映射。未跑通的 live-model 不会被写成已通过。</p>
        <label class="saved">已保存策略
          <select aria-label="已保存策略" :value="store.strategyId" @change="store.openStrategy($event.target.value)">
            <option value="">新策略</option>
            <option v-for="s in store.strategies" :key="s.strategy_id" :value="s.strategy_id">{{ s.name }}</option>
          </select>
        </label>
        <textarea v-model="store.prompt" maxlength="4000" rows="5" placeholder="例如：MACD 金叉且 KDJ 的 K 小于 30 时半仓买入，MACD 死叉卖出；收盘亏损达到 5% 也卖出。"></textarea>
        <div class="actions">
          <button type="button" class="zg-btn zg-btn-primary" data-testid="generate-rule" :aria-busy="ui.genStatus === 'generating' || ui.genStatus === 'queued'" @click="onGenerate">生成规则</button>
          <button v-if="ui.genStatus === 'queued' || ui.genStatus === 'generating'" type="button" class="zg-btn zg-btn-ghost" @click="store.cancelGenerate()">取消生成</button>
        </div>
        <p data-testid="gen-status" class="meta">状态 {{ ui.genStatus || 'idle' }}</p>
        <p v-if="ui.genError" class="err">{{ ui.genError }}</p>
        <div v-if="questions.length" class="card">
          <h3>需要澄清</h3>
          <ul><li v-for="q in questions" :key="q">{{ q }}</li></ul>
        </div>
        <div class="card" data-testid="rule-card" :key="ui.draftRev">
          <template v-if="ui.draft && ui.draft.dsl">
            <h3>{{ ui.draft.dsl.name || '规则卡' }}</h3>
            <p>标的 {{ ui.draft.dsl.instrument_id }} · {{ ui.draft.dsl.signal_period }} · {{ ui.draft.dsl.price_basis }}</p>
            <p data-testid="position-value">仓位 {{ ui.draft.dsl.position?.value }} · 止损 {{ ui.draft.dsl.risk?.stop_loss_pct || '未启用' }}</p>
            <p>执行 {{ ui.draft.dsl.execution?.timing }}（历史模拟，次一交易日开盘）</p>
            <label v-if="kdjK !== ''">KDJ K 阈值
              <input :value="kdjK" type="number" min="0" max="100" @change="onK($event)">
            </label>
            <ul v-if="assumptions.length">
              <li v-for="a in assumptions" :key="a">{{ a }}</li>
            </ul>
            <div class="actions">
              <button type="button" class="zg-btn zg-btn-ghost" @click="onApply">应用修改</button>
              <button type="button" class="zg-btn zg-btn-ghost" :disabled="store.saveBusy" @click="store.saveCurrent()">保存策略</button>
              <button type="button" class="zg-btn zg-btn-ghost" @click="store.showAdvanced = !store.showAdvanced">{{ store.showAdvanced ? '隐藏 DSL' : '查看 DSL' }}</button>
            </div>
            <p v-if="store.saveNotice" class="meta">{{ store.saveNotice }}</p>
            <p v-if="store.saveError" class="err">{{ store.saveError }}</p>
            <textarea v-if="store.showAdvanced" v-model="store.dslText" class="dsl" rows="12" aria-label="策略 DSL"></textarea>
          </template>
          <p v-else class="hint">生成规则后，这里显示可编辑的入场、退出、仓位和假设。</p>
        </div>
        <div class="card">
          <h3>回测配置</h3>
          <label>起始 <input v-model="store.start" type="date"></label>
          <label>结束 <input v-model="store.end" type="date"></label>
          <label>初始资金 <input v-model="store.initialCash"></label>
          <label>滑点 bps <input v-model="store.slippage"></label>
          <p class="hint">佣金按产品默认假设 2.5bp，不是你的券商费率。费用未知时不会填 0。</p>
          <div class="actions">
            <button type="button" class="zg-btn zg-btn-primary" data-testid="run-backtest" :disabled="ui.btBusy || !ui.draft?.dsl" @click="onBacktest">运行回测</button>
            <button v-if="ui.btBusy" type="button" class="zg-btn zg-btn-ghost" @click="store.cancelBacktest()">取消回测</button>
          </div>
        </div>
      </aside>
    </div>
    <section v-show="!mobile || store.mobileTab === 'result'" class="results" data-testid="backtest-results">
      <h2>回测结果 · 历史模拟</h2>
      <p v-if="ui.btBusy">正在计算账本</p>
      <p v-else-if="ui.btError" class="err">{{ ui.btError }}</p>
      <p v-else-if="!ui.results">运行回测后查看净值、回撤、成交与未成交原因。</p>
      <template v-else>
        <div class="seg" role="tablist" aria-label="回测结果">
          <button type="button" :class="{ on: store.resultTab === 'overview' }" @click="store.resultTab = 'overview'">概览</button>
          <button type="button" :class="{ on: store.resultTab === 'equity' }" @click="store.resultTab = 'equity'">净值回撤</button>
          <button type="button" :class="{ on: store.resultTab === 'trades' }" @click="store.resultTab = 'trades'">成交</button>
          <button type="button" :class="{ on: store.resultTab === 'signals' }" @click="store.resultTab = 'signals'">信号</button>
          <button type="button" :class="{ on: store.resultTab === 'notes' }" @click="store.resultTab = 'notes'">运行说明</button>
        </div>
        <template v-if="store.resultTab === 'overview'">
          <dl class="metrics">
            <div><dt>总收益</dt><dd>{{ ui.results.metrics?.total_return ?? '—' }}</dd></div>
            <div><dt>最大回撤</dt><dd>{{ ui.results.metrics?.max_drawdown ?? '—' }}</dd></div>
            <div><dt>胜率</dt><dd>{{ ui.results.metrics?.win_rate ?? '无已完成交易' }}</dd></div>
            <div><dt>无风险利率</dt><dd>{{ ui.results.metrics?.rf }}</dd></div>
          </dl>
          <p class="hint">基准为同一股票买入持有。结果 hash {{ ui.results.result_hash?.slice(0, 12) }}</p>
          <button type="button" class="zg-btn zg-btn-ghost" @click="store.exportCSV">导出成交 CSV</button>
        </template>
        <template v-else-if="store.resultTab === 'equity'">
          <table v-if="ui.results.equity?.length">
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
          <table v-if="ui.trades?.fills?.length">
            <thead><tr><th>日期</th><th>数量</th><th>价格</th><th>现金变化</th></tr></thead>
            <tbody>
              <tr v-for="f in ui.trades.fills" :key="f.id">
                <td>{{ f.fill_date }}</td><td>{{ f.qty }}</td><td>{{ f.price }}</td><td>{{ f.cash_delta }}</td>
              </tr>
            </tbody>
          </table>
          <p v-else>没有成交。信号不等于交易。</p>
          <h3>订单与未成交原因</h3>
          <table v-if="ui.trades?.orders?.length">
            <thead><tr><th>提交日</th><th>方向</th><th>状态</th><th>原因</th></tr></thead>
            <tbody>
              <tr v-for="o in ui.trades.orders" :key="o.id">
                <td>{{ o.submitted_date }}</td><td>{{ o.side }}</td><td>{{ o.status }}</td><td>{{ o.reason || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </template>
        <template v-else-if="store.resultTab === 'signals'">
          <table v-if="signalRows.length">
            <thead><tr><th>日期</th><th>类型</th></tr></thead>
            <tbody>
              <tr v-for="s in signalRows" :key="s.date + s.kind">
                <td>{{ s.date }}</td><td>{{ s.kind }}</td>
              </tr>
            </tbody>
          </table>
          <p v-else class="hint">没有信号。</p>
        </template>
        <ul v-else>
          <li v-for="a in resultAssumptions" :key="a">{{ a }}</li>
        </ul>
      </template>
    </section>
  </div>
</template>
<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import InstrumentSearch from '../../components/strategies/InstrumentSearch.vue'
import QuoteStrip from '../../components/strategies/QuoteStrip.vue'
import ChartPane from '../../components/strategies/ChartPane.vue'
import { useStrategyWorkspace } from '../../stores/strategyWorkspace.js'

const store = useStrategyWorkspace()
const ui = reactive({
  genStatus: '',
  genError: '',
  draft: null,
  draftRev: 0,
  results: null,
  trades: null,
  btBusy: false,
  btError: ''
})
const kdjK = computed(() => store.kdjK)
function syncFromStore() {
  ui.genStatus = store.genStatus
  ui.genError = store.genError
  ui.draft = store.draft
  ui.draftRev = store.draftRev
  ui.results = store.results
  ui.trades = store.trades
  ui.btBusy = store.btBusy
  ui.btError = store.btError
}
const width = ref(typeof window === 'undefined' ? 1440 : window.innerWidth)
const mobile = computed(() => width.value < 768)
const narrow = computed(() => width.value < 1100)
const drawer = computed(() => !mobile.value && narrow.value && store.sidebarOpen)
const periods = ['1d', '1w', '1mo']
const periodLabel = { '1d': '日', '1w': '周', '1mo': '月' }
const adjusts = ['raw', 'qfq', 'hfq']
const freshness = computed(() => store.ohlcvMeta?.quality?.freshness_status === 'fresh' ? '日线已完成' : '时效未验证 · 日线底座')
const questions = computed(() => {
  const q = store.draft?.clarification
  return Array.isArray(q) ? q : []
})
const assumptions = computed(() => {
  const a = store.draft?.assumptions
  return Array.isArray(a) ? a : []
})
const resultAssumptions = computed(() => {
  const a = store.results?.assumptions
  return Array.isArray(a) ? a : []
})
const equityTail = computed(() => {
  const rows = store.results?.equity || []
  return rows.slice(-12)
})
const signalRows = computed(() => {
  const s = store.results?.signals
  return Array.isArray(s) ? s : []
})
function addInd(ev) {
  const type = ev.target.value
  ev.target.value = ''
  if (!type) return
  store.addIndicator(type)
}
async function onGenerate() {
  ui.genStatus = 'queued'
  const out = await store.generate()
  syncFromStore()
  await nextTick()
  return out
}
async function onApply() {
  await store.applyDraft()
  syncFromStore()
  await nextTick()
}
async function onBacktest() {
  ui.btBusy = true
  await store.runBacktest()
  syncFromStore()
  await nextTick()
}
function onK(ev) {
  store.setKdjK(ev.target.value)
}
function onResize() { width.value = window.innerWidth }
onMounted(() => {
  store.boot()
  window.__zgGenerate = onGenerate
  window.addEventListener('resize', onResize)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
})
</script>
<style scoped>
.workbench {
  flex: 1; min-height: 0; display: flex; flex-direction: column; position: relative;
  background: var(--zg-paper); color: var(--zg-ink);
}
.toolbar {
  display: flex; gap: 12px; align-items: center; flex-wrap: wrap;
  padding: 8px 16px; border-bottom: 1px solid var(--zg-line); background: var(--zg-surface);
}
.ident { margin: 0; display: flex; flex-direction: column; font-size: 13px; }
.ident span { color: var(--zg-text-secondary); }
.stale { font-size: 11px; }
.seg { display: flex; gap: 4px; }
.seg .on, .seg button.on { background: var(--zg-surface-muted); }
.tabs { display: flex; border-bottom: 1px solid var(--zg-line); }
.tabs button {
  flex: 1; height: 40px; border: 0; background: transparent; font: inherit; cursor: pointer;
}
.tabs .on { box-shadow: inset 0 -2px 0 var(--zg-action); }
.body { flex: 1; min-height: 0; display: flex; }
.chart-col { flex: 1; min-width: 0; display: flex; flex-direction: column; position: relative; z-index: 1; overflow: hidden; }
.side {
  width: 360px; flex: none; overflow: auto; padding: 16px; position: relative; z-index: 2;
  border-left: 1px solid var(--zg-line); background: var(--zg-surface); display: grid; gap: 12px; align-content: start;
}
.side.drawer { position: absolute; right: 0; top: 0; bottom: 0; z-index: 15; box-shadow: var(--zg-shadow-overlay); }
h2, h3 { margin: 0; font-size: 16px; font-family: var(--zg-font-editorial); }
textarea, input, select {
  width: 100%; border: 1px solid var(--zg-control-border); border-radius: 6px; padding: 8px; font: inherit; background: #fff;
}
.chip input { width: 52px; padding: 2px 4px; }
.hint, .meta { margin: 0; font-size: 12px; color: var(--zg-text-secondary); }
.err, .state.err { color: var(--zg-error-fg); }
.card { border: 1px solid var(--zg-line); border-radius: 10px; padding: 12px; display: grid; gap: 8px; }
.dsl { font-family: var(--zg-font-number); font-size: 11px; }
.actions { display: flex; flex-wrap: wrap; gap: 8px; }
.results {
  border-top: 1px solid var(--zg-line); padding: 12px 16px; max-height: 280px; overflow: auto; background: var(--zg-surface);
}
.metrics { display: flex; gap: 24px; margin: 8px 0; }
dt { font-size: 12px; color: var(--zg-text-secondary); }
dd { margin: 0; font-family: var(--zg-font-number); }
table { width: 100%; border-collapse: collapse; font-size: 12px; font-family: var(--zg-font-number); }
th, td { text-align: left; padding: 4px 8px; border-bottom: 1px solid var(--zg-line); }
.ind-dock {
  display: flex; gap: 8px; align-items: center; flex-wrap: wrap; padding: 6px 12px; border-top: 1px solid var(--zg-line); font-size: 12px;
}
.chip { display: inline-flex; gap: 6px; align-items: center; border: 1px solid var(--zg-line); background: var(--zg-surface-muted); border-radius: 999px; padding: 2px 8px; }
.chip button { border: 0; background: transparent; cursor: pointer; }
.state { margin: 24px; color: var(--zg-text-secondary); }
.saved { display: grid; gap: 4px; font-size: 12px; }
.is-mobile .side { width: 100%; border-left: 0; }
.is-mobile .results { max-height: none; flex: 1; }
</style>
