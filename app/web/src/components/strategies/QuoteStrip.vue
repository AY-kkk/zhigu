<template>
  <section class="quote" aria-live="polite" data-testid="quote-strip">
    <dl v-if="quote?.last" class="last">
      <div><dt>最新</dt><dd :class="lastTone" data-testid="quote-last">{{ lastSign }} {{ quote.last }}</dd></div>
      <div v-if="quote.change"><dt>涨跌</dt><dd :class="lastTone">{{ quote.change }}</dd></div>
      <div v-if="quote.change_pct"><dt>涨跌幅</dt><dd :class="lastTone">{{ quote.change_pct }}%</dd></div>
      <div v-if="quote.volume"><dt>现量</dt><dd>{{ quote.volume }} 股</dd></div>
    </dl>
    <p v-if="!bar" class="muted">光标移到 K 线上可看开高低收。</p>
    <dl v-else>
      <div><dt>日期</dt><dd data-testid="quote-date">{{ timeKey(bar.time) }}</dd></div>
      <div><dt>开</dt><dd :class="tone">{{ bar.open }}</dd></div>
      <div><dt>高</dt><dd>{{ bar.high }}</dd></div>
      <div><dt>低</dt><dd>{{ bar.low }}</dd></div>
      <div><dt>收</dt><dd :class="tone">{{ sign }} {{ bar.close }}</dd></div>
      <div><dt>量</dt><dd>{{ bar.volume }} 股</dd></div>
    </dl>
    <button v-if="bar" type="button" class="zg-btn zg-btn-ghost copy" @click="copy">复制数值</button>
  </section>
</template>
<script setup>
import { computed } from 'vue'
import { timeKey } from './chartTime.js'
const props = defineProps({
  bar: { type: Object, default: null },
  quote: { type: Object, default: null },
  values: { type: Object, default: () => ({}) }
})
const up = computed(() => props.bar && Number(props.bar.close) >= Number(props.bar.open))
const tone = computed(() => (up.value ? 'up' : 'down'))
const sign = computed(() => (up.value ? '▲' : '▼'))
const lastUp = computed(() => props.quote && Number(props.quote.change) >= 0)
const lastTone = computed(() => (lastUp.value ? 'up' : 'down'))
const lastSign = computed(() => (lastUp.value ? '▲' : '▼'))
async function copy() {
  const b = props.bar
  const extras = Object.entries(props.values || {}).map(([k, v]) => `${k}${v}`).join(' ')
  const q = props.quote?.last ? ` 最新${props.quote.last}` : ''
  const text = `${timeKey(b.time)} 开${b.open} 高${b.high} 低${b.low} 收${b.close} 量${b.volume}${q}${extras ? ` ${extras}` : ''}`
  await navigator.clipboard.writeText(text)
}
</script>
<style scoped>
.quote {
  display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
  padding: 6px 12px; border-bottom: 1px solid var(--zg-line); font-family: var(--zg-font-number); font-size: 12px;
}
dl { display: flex; gap: 16px; margin: 0; flex-wrap: wrap; }
.last { padding-right: 12px; border-right: 1px solid var(--zg-line); }
div { display: flex; gap: 6px; }
dt { color: var(--zg-text-secondary); }
.up { color: #E85858; }
.down { color: #2E7D32; }
.muted { margin: 0; color: var(--zg-text-secondary); }
.copy { min-height: 32px; font-size: 12px; }
</style>
