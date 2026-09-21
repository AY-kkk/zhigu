<template>
  <div class="chart-root">
    <div ref="host" class="chart-host" role="img" :aria-label="label"></div>
    <p class="attr">图表由 Lightweight Charts 渲染，不负责指标或回测计算。</p>
  </div>
</template>
<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { CandlestickSeries, ColorType, createChart, createSeriesMarkers, HistogramSeries, LineSeries } from 'lightweight-charts'

const props = defineProps({
  bars: { type: Array, default: () => [] },
  series: { type: Array, default: () => [] },
  fills: { type: Array, default: () => [] },
  signals: { type: Array, default: () => [] }
})
const emit = defineEmits(['crosshair'])
const host = ref(null)
const label = computed(() => `K 线 ${props.bars.length} 根`)
let chart
let candle
let volume
let markersApi
let ro

function toNum(v) {
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

function paint() {
  if (!chart || !candle) return
  try {
  const data = props.bars.map((b) => ({
    time: b.time,
    open: toNum(b.open),
    high: toNum(b.high),
    low: toNum(b.low),
    close: toNum(b.close)
  })).filter((b) => b.open != null)
  candle.setData(data)
  if (volume) {
    volume.setData(props.bars.map((b) => ({
      time: b.time,
      value: toNum(b.volume) || 0,
      color: Number(b.close) >= Number(b.open) ? 'rgba(232,88,88,0.45)' : 'rgba(46,125,50,0.45)'
    })))
  }
  const extras = chart._zgLines || []
  extras.forEach((s) => chart.removeSeries(s))
  chart._zgLines = []
  props.series.forEach((row, idx) => {
    const color = ['#1d4ed8', '#b45309', '#5b5193', '#0f766e'][idx % 4]
    Object.entries(row.fields || {}).forEach(([name, pts]) => {
      if (name === 'volume' || name === 'hist') return
      try {
        const line = chart.addSeries(LineSeries, { color, lineWidth: 1, priceScaleId: row.type === 'MA' || row.type === 'EMA' || row.type === 'BOLL' ? 'right' : name, title: `${row.id}.${name}` }, row.type === 'MA' || row.type === 'BOLL' || row.type === 'EMA' ? 0 : 2)
        const mapped = (pts || []).map((v, i) => (v == null || !props.bars[i] ? null : { time: props.bars[i].time, value: toNum(v) })).filter(Boolean)
        line.setData(mapped)
        chart._zgLines.push(line)
      } catch {
        /* pane/scale mismatch is non-fatal */
      }
    })
  })
  const markers = []
  ;(props.signals || []).forEach((s) => {
    markers.push({ time: s.date || s.Date, position: s.kind === 'entry' ? 'belowBar' : 'aboveBar', color: '#666', shape: 'circle', text: s.kind === 'entry' ? '信号买' : '信号卖' })
  })
  ;(props.fills || []).forEach((f) => {
    const side = (f.payload && f.payload.Side) || ''
    markers.push({ time: f.fill_date || f.FillDate, position: 'inBar', color: '#171717', shape: 'square', text: '模拟成交' })
    void side
  })
  if (markers.length) {
    if (!markersApi) markersApi = createSeriesMarkers(candle, [])
    markersApi.setMarkers(markers.sort((a, b) => String(a.time).localeCompare(String(b.time))))
  } else if (markersApi) {
    markersApi.setMarkers([])
  }
  } catch {
    /* chart library errors must not abort Vue updates */
  }
}

onMounted(() => {
  chart = createChart(host.value, {
    autoSize: true,
    layout: { background: { type: ColorType.Solid, color: '#FFFEFB' }, textColor: '#171717', fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace', attributionLogo: true },
    grid: { vertLines: { color: '#EEECE7' }, horzLines: { color: '#EEECE7' } },
    rightPriceScale: { borderColor: '#DEDBD4' },
    timeScale: { borderColor: '#DEDBD4', timeVisible: false },
    crosshair: { mode: 1 }
  })
  candle = chart.addSeries(CandlestickSeries, {
    upColor: '#E85858', downColor: '#2E7D32', borderUpColor: '#E85858', borderDownColor: '#2E7D32',
    wickUpColor: '#E85858', wickDownColor: '#2E7D32'
  })
  try {
    volume = chart.addSeries(HistogramSeries, { priceFormat: { type: 'volume' } }, 1)
    const pane = chart.panes?.()[1]
    pane?.priceScale('').applyOptions({ scaleMargins: { top: 0.8, bottom: 0 } })
  } catch {
    try {
      volume = chart.addSeries(HistogramSeries, { priceFormat: { type: 'volume' } })
    } catch {
      volume = null
    }
  }
  chart.subscribeCrosshairMove((param) => {
    if (!param?.time) return
    const row = props.bars.find((b) => b.time === param.time)
    if (row) emit('crosshair', row)
  })
  ro = new ResizeObserver(() => {
    try { chart?.applyOptions({}) } catch { /* ignore */ }
  })
  ro.observe(host.value)
  try { paint() } catch { /* first paint can race layout */ }
})
watch(() => [props.bars, props.series, props.fills, props.signals], () => {
  try { paint() } catch { /* ignore */ }
}, { deep: true })
onBeforeUnmount(() => {
  ro?.disconnect()
  chart?.remove()
  chart = null
})
</script>
<style scoped>
.chart-root { display: flex; flex-direction: column; min-height: 0; flex: 1; }
.chart-host { flex: 1; min-height: 220px; }
.attr { margin: 0; padding: 4px 8px; font-size: 11px; color: #666; }
</style>
