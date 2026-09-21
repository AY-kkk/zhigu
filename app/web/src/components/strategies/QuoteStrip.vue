<template>
  <section class="quote" aria-live="polite">
    <p v-if="!bar" class="muted">将十字光标移到 K 线上，这里显示可复制的日期、价格和成交量。</p>
    <dl v-else>
      <div><dt>日期</dt><dd>{{ bar.time }}</dd></div>
      <div><dt>开</dt><dd :class="tone">{{ bar.open }}</dd></div>
      <div><dt>高</dt><dd>{{ bar.high }}</dd></div>
      <div><dt>低</dt><dd>{{ bar.low }}</dd></div>
      <div><dt>收</dt><dd :class="tone">{{ sign }} {{ bar.close }}</dd></div>
      <div><dt>量</dt><dd>{{ bar.volume }} 股</dd></div>
      <div v-for="(v, k) in values" :key="k"><dt>{{ k }}</dt><dd>{{ v }}</dd></div>
    </dl>
    <button v-if="bar" type="button" class="zg-btn zg-btn-ghost copy" @click="copy">复制数值</button>
  </section>
</template>
<script setup>
import { computed } from 'vue'
const props = defineProps({ bar: { type: Object, default: null }, values: { type: Object, default: () => ({}) } })
const up = computed(() => props.bar && Number(props.bar.close) >= Number(props.bar.open))
const tone = computed(() => (up.value ? 'up' : 'down'))
const sign = computed(() => (up.value ? '▲' : '▼'))
async function copy() {
  const b = props.bar
  const text = `${b.time} 开${b.open} 高${b.high} 低${b.low} 收${b.close} 量${b.volume}`
  await navigator.clipboard.writeText(text)
}
</script>
<style scoped>
.quote {
  display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
  padding: 6px 12px; border-bottom: 1px solid var(--zg-line); font-family: var(--zg-font-number); font-size: 12px;
}
dl { display: flex; gap: 16px; margin: 0; flex-wrap: wrap; }
div { display: flex; gap: 6px; }
dt { color: var(--zg-text-secondary); }
.up { color: #E85858; }
.down { color: #2E7D32; }
.muted { margin: 0; color: var(--zg-text-secondary); }
.copy { min-height: 32px; font-size: 12px; }
</style>
