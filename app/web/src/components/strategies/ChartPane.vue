<template>
  <div class="chart-root">
    <div class="chart-bar">
      <span class="range" data-testid="chart-range">{{ rangeText }}</span>
      <button type="button" class="zg-btn zg-btn-ghost" data-testid="chart-reset" @click="resetView">复位视图</button>
    </div>
    <div class="chart-stage">
      <div ref="host" class="chart-host" data-testid="chart-host" role="img" :aria-label="label"></div>
      <div class="legend" data-testid="chart-legend">
        <span v-for="item in overlayLegend" :key="item.key" :style="{ color: item.color }">{{ item.name }} {{ item.value }}</span>
      </div>
      <div class="pane-tags" aria-hidden="true">
        <span v-for="tag in paneLabels" :key="tag.key" class="pane-tag" :style="{ top: tag.top + 'px' }">{{ tag.text }}</span>
      </div>
    </div>
  </div>
</template>
<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { CandlestickSeries, ColorType, CrosshairMode, HistogramSeries, LineSeries, createChart, createSeriesMarkers } from 'lightweight-charts'
import { timeKey } from './chartTime.js'
import { fillSide } from './tradeSide.js'

const props = defineProps({
  bars: { type: Array, default: () => [] },
  series: { type: Array, default: () => [] },
  indicators: { type: Array, default: () => [] },
  fills: { type: Array, default: () => [] },
  orders: { type: Array, default: () => [] },
  signals: { type: Array, default: () => [] },
  hasMore: { type: Boolean, default: false },
  loadingMore: { type: Boolean, default: false },
  precision: { type: Number, default: 2 }
})
const emit = defineEmits(['crosshair', 'need-older'])
const host = ref(null)
const rangeText = ref('—')
const hoverKey = ref('')
const paneTops = ref([])
const label = computed(() => `K 线 ${props.bars.length} 根，主图叠加均线，副图分栏`)

const overlayTypes = new Set(['MA', 'EMA', 'BOLL'])
const overlayPalette = ['#1d4ed8', '#b45309', '#9f1239', '#0f766e', '#5b5193', '#9a3412']
const fieldColor = {
  mid: '#5b5193', upper: '#0f766e', lower: '#0f766e',
  dif: '#171717', dea: '#b45309', hist: '#9f1239',
  k: '#1d4ed8', d: '#b45309', j: '#9f1239',
  rsi: '#1d4ed8', wr: '#b45309', bias: '#0f766e', cci: '#5b5193', atr: '#b45309',
  obv: '#171717', ma5: '#1d4ed8', ma10: '#b45309'
}

let chart
let candle
let volume
let markersApi
let extras = []
let lastLen = 0
let lastHead = ''
let lastTail = ''
let loadArmed = 0
let layoutKey = ''
let ro

function toNum(v) {
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

function minMove(precision) {
  const p = Math.max(0, Math.min(6, Number(precision) || 2))
  return Number((10 ** -p).toFixed(p))
}

function fmt(v, compact = false) {
  const n = Number(v)
  if (!Number.isFinite(n)) return '—'
  const abs = Math.abs(n)
  if (compact && abs >= 1e8) return `${(n / 1e8).toFixed(2)}亿`
  if (compact && abs >= 1e4) return `${(n / 1e4).toFixed(1)}万`
  const digits = abs >= 100 ? 2 : 3
  return n.toFixed(digits)
}

function findBar(time) {
  const key = timeKey(time)
  if (!key) return null
  return props.bars.find((b) => timeKey(b.time) === key) || null
}

function mapField(pts) {
  const out = []
  for (let i = 0; i < props.bars.length; i += 1) {
    const v = toNum(pts?.[i])
    if (v == null) continue
    out.push({ time: timeKey(props.bars[i].time), value: v })
  }
  return out
}

function buildPlan(indicators) {
  const plan = [{ kind: 'price', key: 'price' }]
  const list = Array.isArray(indicators) ? indicators : []
  if (list.some((row) => String(row?.type || '').toUpperCase() === 'VOL')) {
    plan.push({ kind: 'volume', key: 'volume' })
  }
  list.forEach((row) => {
    const type = String(row?.type || '').toUpperCase()
    if (!type || type === 'VOL' || overlayTypes.has(type)) return
    plan.push({ kind: 'osc', key: `osc:${row.id}`, id: row.id, type })
  })
  return plan
}

function paneFor(row) {
  const type = String(row?.type || '').toUpperCase()
  const plan = buildPlan(props.indicators)
  if (overlayTypes.has(type)) return 0
  if (type === 'VOL') {
    const i = plan.findIndex((p) => p.kind === 'volume')
    return i < 0 ? 0 : i
  }
  let i = plan.findIndex((p) => p.kind === 'osc' && p.id === row.id)
  if (i < 0) i = plan.findIndex((p) => p.kind === 'osc' && p.type === type)
  return i < 0 ? 0 : i
}

function shortName(row, name) {
  const type = String(row?.type || '').toUpperCase()
  const n = row?.params?.n
  if (type === 'MA') return `MA${n || ''}`
  if (type === 'EMA') return `EMA${n || ''}`
  if (type === 'BOLL') return { mid: '中轨', upper: '上轨', lower: '下轨' }[name] || name
  if (type === 'VOL') return name === 'ma5' ? '量MA5' : name === 'ma10' ? '量MA10' : name
  const map = { dif: 'DIF', dea: 'DEA', hist: 'MACD', k: 'K', d: 'D', j: 'J' }
  return map[name] || name.toUpperCase()
}

function colorFor(row, name, idx) {
  const type = String(row?.type || '').toUpperCase()
  if (type === 'MA' || type === 'EMA') {
    const overs = (props.indicators || []).filter((item) => overlayTypes.has(String(item?.type || '').toUpperCase()))
    const at = overs.findIndex((item) => item.id === row.id)
    return overlayPalette[(at < 0 ? idx : at) % overlayPalette.length]
  }
  return fieldColor[name] || overlayPalette[idx % overlayPalette.length]
}

const barIndex = computed(() => {
  const key = hoverKey.value || timeKey(props.bars[props.bars.length - 1]?.time)
  return props.bars.findIndex((b) => timeKey(b.time) === key)
})

const overlayLegend = computed(() => {
  const i = barIndex.value
  const items = []
  ;(props.series || []).forEach((row, idx) => {
    const type = String(row?.type || '').toUpperCase()
    if (!overlayTypes.has(type)) return
    Object.entries(row.fields || {}).forEach(([name, pts]) => {
      items.push({
        key: `${row.id}.${name}`,
        name: shortName(row, name),
        value: i >= 0 && pts?.[i] != null && pts[i] !== '' ? fmt(pts[i]) : '—',
        color: colorFor(row, name, idx)
      })
    })
  })
  return items
})

const paneLabels = computed(() => {
  const plan = buildPlan(props.indicators)
  const i = barIndex.value
  return plan.map((pane, idx) => {
    if (pane.kind === 'price') return null
    let text = pane.kind === 'volume' ? '成交量' : pane.type
    const row = (props.series || []).find((item) => {
      const type = String(item?.type || '').toUpperCase()
      if (pane.kind === 'volume') return type === 'VOL'
      return item.id === pane.id || type === pane.type
    })
    if (row && i >= 0) {
      const bits = Object.entries(row.fields || {})
        .filter(([name]) => name !== 'volume')
        .map(([name, pts]) => `${shortName(row, name)} ${pts?.[i] != null && pts[i] !== '' ? fmt(pts[i], pane.kind === 'volume') : '—'}`)
      if (bits.length) text = `${text}  ${bits.join('  ')}`
    }
    return { key: pane.key, text, top: (paneTops.value[idx] || 0) + 4 }
  }).filter(Boolean)
})

function defaultRange() {
  if (!chart) return
  const n = props.bars.length
  if (n <= 80) {
    chart.timeScale().fitContent()
    return
  }
  chart.timeScale().setVisibleLogicalRange({ from: n - 120, to: n + 3 })
}

function resetView() {
  defaultRange()
}

function paneList() {
  try { return chart?.panes?.() || [] } catch { return [] }
}

function addOnPane(idx, def, opts) {
  const panes = paneList()
  const pane = panes[idx]
  if (pane?.addSeries) return pane.addSeries(def, opts)
  const created = chart.addSeries(def, opts)
  try { created.moveToPane(idx) } catch { /* keep default pane */ }
  return created
}

function layoutPanes() {
  const panes = paneList()
  const tops = []
  let y = 0
  panes.forEach((p, i) => {
    try {
      p.setPreserveEmptyPane(true)
      p.setStretchFactor(i === 0 ? 2.6 : 1)
    } catch { /* stretch is optional */ }
    let h = 0
    try { h = p.getHeight() } catch { h = 0 }
    tops.push(y)
    y += h
  })
  paneTops.value = tops
  if (host.value) {
    const min = Math.max(420, 220 + Math.max(0, panes.length - 1) * 108)
    host.value.style.minHeight = `${min}px`
    host.value.dataset.paneCount = String(panes.length)
    host.value.dataset.paneHeights = panes.map((p) => {
      try { return p.getHeight() } catch { return 0 }
    }).join(',')
    host.value.dataset.paneSeries = panes.map((p) => {
      try { return (p.getSeries() || []).map((s) => s.seriesType()).join('+') } catch { return '' }
    }).join('|')
  }
}

let lastBox = ''
function resizeChart() {
  if (!chart || !host.value) return
  const box = `${host.value.clientWidth}x${host.value.clientHeight}`
  if (box !== lastBox) {
    lastBox = box
    try { chart.resize(host.value.clientWidth, host.value.clientHeight, true) } catch { /* ignore */ }
  }
  requestAnimationFrame(() => layoutPanes())
}

function syncPanes() {
  const plan = buildPlan(props.indicators)
  const key = plan.map((p) => p.key).join('|')
  if (key === layoutKey && candle) return false
  layoutKey = key
  extras.forEach((s) => {
    try { chart.removeSeries(s) } catch { /* already gone */ }
  })
  extras = []
  if (volume) {
    try { chart.removeSeries(volume) } catch { /* already gone */ }
    volume = null
  }
  while (paneList().length > 1) {
    try { chart.removePane(paneList().length - 1) } catch { break }
  }
  while (paneList().length < plan.length) {
    try { chart.addPane(true) } catch { break }
  }
  const volIdx = plan.findIndex((p) => p.kind === 'volume')
  if (volIdx > 0) {
    try {
      volume = addOnPane(volIdx, HistogramSeries, {
        priceFormat: { type: 'volume' },
        lastValueVisible: false,
        priceLineVisible: false
      })
    } catch {
      volume = null
    }
  }
  layoutPanes()
  return true
}

function paint() {
  if (!chart || !candle) return
  try {
    const head = props.bars[0]?.time || ''
    const tail = props.bars[props.bars.length - 1]?.time || ''
    const prepend = lastLen > 0 && head && head !== lastHead && tail === lastTail
    const logical = chart.timeScale().getVisibleLogicalRange()
    const added = prepend ? props.bars.length - lastLen : 0
    const rebuilt = syncPanes()

    const data = props.bars.map((b) => ({
      time: timeKey(b.time),
      open: toNum(b.open),
      high: toNum(b.high),
      low: toNum(b.low),
      close: toNum(b.close)
    })).filter((b) => b.open != null && b.time)
    candle.applyOptions({
      priceFormat: { type: 'price', precision: props.precision, minMove: minMove(props.precision) }
    })
    candle.setData(data)
    if (volume) {
      volume.setData(props.bars.map((b) => ({
        time: timeKey(b.time),
        value: toNum(b.volume) || 0,
        color: Number(b.close) >= Number(b.open) ? 'rgba(232,88,88,0.45)' : 'rgba(46,125,50,0.45)'
      })).filter((b) => b.time))
    }

    extras.forEach((s) => {
      try { chart.removeSeries(s) } catch { /* already gone */ }
    })
    extras = []
    props.series.forEach((row, idx) => {
      const pane = paneFor(row)
      Object.entries(row.fields || {}).forEach(([name, pts]) => {
        if (name === 'volume') return
        const mapped = mapField(pts)
        if (!mapped.length) return
        const title = shortName(row, name)
        try {
          let seriesApi
          if (name === 'hist') {
            seriesApi = addOnPane(pane, HistogramSeries, {
              priceFormat: { type: 'price', precision: 3, minMove: 0.001 },
              title,
              priceScaleId: 'right',
              lastValueVisible: true
            })
            seriesApi.setData(mapped.map((p) => ({
              ...p,
              color: p.value >= 0 ? 'rgba(232,88,88,0.7)' : 'rgba(46,125,50,0.7)'
            })))
          } else {
            seriesApi = addOnPane(pane, LineSeries, {
              color: colorFor(row, name, idx),
              lineWidth: 1,
              title,
              priceScaleId: 'right',
              lastValueVisible: true,
              priceLineVisible: false
            })
            seriesApi.setData(mapped)
          }
          extras.push(seriesApi)
        } catch {
          /* pane mismatch is non-fatal */
        }
      })
    })
    layoutPanes()

    const markers = []
    const fills = props.fills || []
    fills.forEach((f) => {
      const time = timeKey(f.fill_date || f.FillDate || f.Date)
      if (!time) return
      const buy = fillSide(f, props.orders) === 'buy'
      markers.push({
        time,
        position: buy ? 'belowBar' : 'aboveBar',
        color: buy ? '#E85858' : '#2E7D32',
        shape: buy ? 'arrowUp' : 'arrowDown',
        text: buy ? '买' : '卖'
      })
    })
    if (!fills.length) {
      ;(props.signals || []).forEach((s) => {
        const time = timeKey(s.date || s.Date)
        if (!time) return
        const buy = s.kind === 'entry'
        markers.push({
          time,
          position: buy ? 'belowBar' : 'aboveBar',
          color: buy ? '#E85858' : '#2E7D32',
          shape: 'circle',
          text: buy ? '买点' : '卖点'
        })
      })
    }
    if (!markersApi) markersApi = createSeriesMarkers(candle, [])
    markersApi.setMarkers(markers.sort((a, b) => String(a.time).localeCompare(String(b.time))))

    if (prepend && logical && added > 0) {
      chart.timeScale().setVisibleLogicalRange({ from: logical.from + added, to: logical.to + added })
    } else if (rebuilt && logical && lastHead) {
      chart.timeScale().setVisibleLogicalRange(logical)
    } else if (!lastHead || (head !== lastHead && tail !== lastTail)) {
      defaultRange()
    }
    lastLen = props.bars.length
    lastHead = head
    lastTail = tail
    requestAnimationFrame(() => layoutPanes())
  } catch {
    /* chart library errors must not abort Vue updates */
  }
}

function onVisible(range) {
  if (!range || !props.hasMore || props.loadingMore) return
  if (range.from > 12) {
    loadArmed = 0
    return
  }
  const now = Date.now()
  if (now - loadArmed < 400) return
  loadArmed = now
  emit('need-older')
}

function onTimeRange(range) {
  if (!range) {
    rangeText.value = '—'
    return
  }
  rangeText.value = `${timeKey(range.from) || '—'} — ${timeKey(range.to) || '—'}`
}

onMounted(() => {
  const w = host.value?.clientWidth || 640
  const h = host.value?.clientHeight || 560
  chart = createChart(host.value, {
    width: w,
    height: h,
    autoSize: false,
    layout: {
      background: { type: ColorType.Solid, color: '#FFFEFB' },
      textColor: '#171717',
      fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace',
      attributionLogo: true
    },
    grid: { vertLines: { color: '#EEECE7' }, horzLines: { color: '#EEECE7' } },
    rightPriceScale: { borderColor: '#DEDBD4' },
    timeScale: {
      borderColor: '#DEDBD4',
      timeVisible: false,
      rightOffset: 2,
      barSpacing: 8,
      minBarSpacing: 2,
      fixLeftEdge: true,
      fixRightEdge: true,
      lockVisibleTimeRangeOnResize: true,
      shiftVisibleRangeOnNewBar: false
    },
    crosshair: { mode: CrosshairMode.MagnetOHLC },
    handleScroll: { mouseWheel: true, pressedMouseMove: true, horzTouchDrag: true, vertTouchDrag: false },
    handleScale: { axisPressedMouseMove: true, mouseWheel: true, pinch: true },
    kineticScroll: { mouse: true, touch: true }
  })
  candle = chart.addSeries(CandlestickSeries, {
    upColor: '#E85858', downColor: '#2E7D32', borderUpColor: '#E85858', borderDownColor: '#2E7D32',
    wickUpColor: '#E85858', wickDownColor: '#2E7D32',
    lastValueVisible: false, priceLineVisible: false
  })
  layoutPanes()
  chart.subscribeCrosshairMove((param) => {
    let row = findBar(param?.time)
    if (!row && Number.isFinite(param?.logical)) {
      const i = Math.round(param.logical)
      row = props.bars[i]
    }
    if (!row) return
    hoverKey.value = timeKey(row.time)
    emit('crosshair', row)
  })
  chart.timeScale().subscribeVisibleLogicalRangeChange(onVisible)
  chart.timeScale().subscribeVisibleTimeRangeChange(onTimeRange)
  if (typeof ResizeObserver !== 'undefined' && host.value) {
    ro = new ResizeObserver(() => resizeChart())
    ro.observe(host.value)
  }
  try { paint() } catch { /* first paint can race layout */ }
})
watch(() => [props.bars, props.series, props.fills, props.signals, props.orders, props.indicators, props.precision], () => {
  try { paint() } catch { /* ignore */ }
}, { deep: true })
onBeforeUnmount(() => {
  try { ro?.disconnect() } catch { /* ignore */ }
  ro = null
  try { chart?.timeScale().unsubscribeVisibleLogicalRangeChange(onVisible) } catch { /* ignore */ }
  try { chart?.timeScale().unsubscribeVisibleTimeRangeChange(onTimeRange) } catch { /* ignore */ }
  chart?.remove()
  chart = null
  candle = null
  volume = null
  markersApi = null
  extras = []
  layoutKey = ''
})
defineExpose({ resetView })
</script>
<style scoped>
.chart-root { display: flex; flex-direction: column; min-height: 0; flex: 1 0 auto; }
.chart-bar {
  display: flex; align-items: center; gap: 10px; flex: none;
  padding: 2px 12px; border-bottom: 1px solid var(--zg-line); font-size: 12px;
}
.chart-bar .zg-btn { min-height: 28px; padding: 0 10px; font-size: 12px; }
.range { font-family: var(--zg-font-number); color: var(--zg-ink); }
.chart-stage { position: relative; flex: 1 0 auto; min-height: 0; display: flex; }
.chart-host { flex: 1 0 auto; min-height: 420px; width: 100%; touch-action: none; overscroll-behavior: contain; }
.legend {
  position: absolute; z-index: 2; left: 8px; top: 4px; right: 64px;
  display: flex; gap: 12px; overflow: hidden; white-space: nowrap; pointer-events: none;
  font-family: var(--zg-font-number); font-size: 12px; line-height: 18px;
}
.pane-tags { position: absolute; inset: 0; pointer-events: none; z-index: 2; }
.pane-tag {
  position: absolute; left: 8px; max-width: calc(100% - 80px);
  overflow: hidden; white-space: nowrap; text-overflow: ellipsis;
  font-family: var(--zg-font-number); font-size: 11px; line-height: 16px; color: var(--zg-text-secondary);
  background: rgba(255, 254, 251, 0.88); padding: 0 4px;
}
</style>
