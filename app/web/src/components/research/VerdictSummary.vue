<template>
  <section v-if="show" class="verdict" :class="kind">
    <div class="icon" aria-hidden="true">
      <ZhiguIcon :name="icon" :size="32" />
    </div>
    <div>
      <h2>{{ title }}</h2>
      <p v-if="summary">{{ summary }}</p>
    </div>
  </section>
</template>
<script setup>
import { computed } from 'vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'
import { VERDICT_LABELS } from '../../utils/researchCopy.js'

const props = defineProps({
  quality: { type: String, default: '' },
  verdict: { type: String, default: '' },
  summary: { type: String, default: '' }
})
const show = computed(() => props.quality !== 'incomplete' && !!props.verdict)
const kind = computed(() => props.verdict || 'insufficient')
const title = computed(() => VERDICT_LABELS[props.verdict] || '判断暂未提供')
const icon = computed(() => {
  if (props.verdict === 'supported') return 'support'
  if (props.verdict === 'challenged') return 'challenge'
  if (props.verdict === 'mixed') return 'mixed'
  return 'insufficient'
})
</script>
<style scoped>
.verdict {
  display: flex;
  gap: 20px;
  padding: 24px;
  border-radius: var(--zg-radius-card);
  border: 1px solid var(--zg-line);
  background: var(--zg-surface);
}
.icon {
  width: 40px;
  height: 40px;
  display: grid;
  place-items: center;
  flex: none;
}
h2 { margin: 0; font-family: var(--zg-font-editorial); font-size: 24px; line-height: 34px; font-weight: 600; }
p { margin: 8px 0 0; font-size: 16px; line-height: 28px; }
.supported { background: var(--zg-support-bg); border-color: #edd5d1; }
.supported .icon { background: var(--zg-support-bg); color: var(--zg-support-fg); }
.challenged { background: var(--zg-challenge-bg); border-color: #d3e3cf; }
.challenged .icon { background: var(--zg-challenge-bg); color: var(--zg-challenge-fg); }
.mixed { background: var(--zg-mixed-bg); border-color: #dcd7ec; }
.mixed .icon { background: var(--zg-mixed-bg); color: var(--zg-mixed-fg); }
.insufficient { border-color: var(--zg-line); background: var(--zg-insufficient-bg); }
.insufficient .icon { background: var(--zg-insufficient-bg); color: var(--zg-insufficient-fg); }
@media (max-width: 767px) { .verdict { padding: 20px 16px; gap: 12px; } .icon { width: 32px; } }
</style>
