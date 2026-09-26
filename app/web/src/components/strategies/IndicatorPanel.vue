<template>
  <aside class="panel" data-testid="indicator-panel" aria-live="polite">
    <p class="date">{{ date || '移动光标' }}</p>
    <p v-if="!groups.length" class="muted">KDJ、MACD 等数值会跟 K 线光标一起变。</p>
    <section v-for="group in groups" :key="group.id">
      <h3>{{ group.type }}</h3>
      <dl>
        <div v-for="field in group.fields" :key="field.name">
          <dt>{{ field.label }}</dt>
          <dd>{{ field.value }}</dd>
        </div>
      </dl>
    </section>
  </aside>
</template>
<script setup>
import { computed } from 'vue'
import { timeKey } from './chartTime.js'

const props = defineProps({
  bar: { type: Object, default: null },
  bars: { type: Array, default: () => [] },
  series: { type: Array, default: () => [] }
})

const labels = {
  ma: 'MA', ema: 'EMA', mid: '中轨', upper: '上轨', lower: '下轨',
  dif: 'DIF', dea: 'DEA', hist: 'MACD',
  k: 'K', d: 'D', j: 'J',
  rsi: 'RSI', wr: 'WR', bias: 'BIAS', cci: 'CCI', atr: 'ATR', obv: 'OBV',
  ma5: 'MA5', ma10: 'MA10', volume: '量'
}

const date = computed(() => timeKey(props.bar?.time))

const index = computed(() => {
  const key = date.value
  if (!key) return -1
  return props.bars.findIndex((b) => timeKey(b.time) === key)
})

function fmt(v, volume) {
  const n = Number(v)
  if (!Number.isFinite(n)) return '—'
  const abs = Math.abs(n)
  if (volume && abs >= 1e8) return `${(n / 1e8).toFixed(2)}亿`
  if (volume && abs >= 1e4) return `${(n / 1e4).toFixed(1)}万`
  return abs >= 100 ? n.toFixed(2) : n.toFixed(3)
}

const groups = computed(() => {
  const i = index.value
  return (props.series || []).map((row) => {
    const type = String(row.type || '').toUpperCase()
    const n = row.params?.n
    const title = (type === 'MA' || type === 'EMA') && n ? `${type}${n}` : type
    const fields = Object.entries(row.fields || {}).map(([name, pts]) => ({
      name,
      label: labels[name] || name.toUpperCase(),
      value: i >= 0 && pts?.[i] != null && pts[i] !== '' ? fmt(pts[i], name === 'volume' || type === 'VOL') : '—'
    }))
    return { id: row.id || title, type: title, fields }
  }).filter((group) => group.fields.length)
})
</script>
<style scoped>
.panel {
  width: 148px; flex: none; overflow: auto;
  border-right: 1px solid var(--zg-line); background: var(--zg-surface);
  padding: 8px 10px; display: grid; gap: 10px; align-content: start;
}
.date { margin: 0; font-family: var(--zg-font-number); font-size: 12px; }
.muted { margin: 0; font-size: 12px; color: var(--zg-text-secondary); line-height: 1.4; }
h3 { margin: 0 0 4px; font-size: 12px; font-weight: 600; }
dl { margin: 0; display: grid; gap: 2px; }
div { display: flex; justify-content: space-between; gap: 8px; font-family: var(--zg-font-number); font-size: 12px; }
dt { color: var(--zg-text-secondary); }
dd { margin: 0; }
</style>
