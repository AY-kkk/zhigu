<template>
  <span class="tag" :class="tone">
    <ZhiguIcon :name="icon" :size="14" />
    <span>{{ label }}</span>
  </span>
</template>
<script setup>
import { computed } from 'vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'
import { VERDICT_LABELS, CLAIM_TYPE_LABELS } from '../../utils/researchCopy.js'

const props = defineProps({
  kind: { type: String, default: '' },
  label: { type: String, default: '' }
})

const tone = computed(() => {
  if (['supported', 'support'].includes(props.kind)) return 'support'
  if (['challenged', 'challenge'].includes(props.kind)) return 'challenge'
  if (props.kind === 'mixed') return 'mixed'
  if (['insufficient', 'unknown'].includes(props.kind)) return 'insufficient'
  return 'neutral'
})
const icon = computed(() => {
  if (tone.value === 'support') return 'support'
  if (tone.value === 'challenge') return 'challenge'
  if (tone.value === 'mixed') return 'mixed'
  if (tone.value === 'insufficient') return 'insufficient'
  if (props.kind === 'fact') return 'report'
  if (props.kind === 'inference') return 'verify'
  return 'info'
})
const label = computed(() => {
  if (props.label) return props.label
  return VERDICT_LABELS[props.kind] || CLAIM_TYPE_LABELS[props.kind] || props.kind
})
</script>
<style scoped>
.tag {
  width: fit-content;
  max-width: 100%;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 24px;
  padding: 0 10px;
  border-radius: var(--zg-radius-tag);
  font-size: 12px;
  line-height: 18px;
  font-weight: 500;
  white-space: nowrap;
}
.support { color: var(--zg-support-fg); background: var(--zg-support-bg); }
.challenge { color: var(--zg-challenge-fg); background: var(--zg-challenge-bg); }
.mixed { color: var(--zg-mixed-fg); background: var(--zg-mixed-bg); }
.insufficient { color: var(--zg-insufficient-fg); background: var(--zg-insufficient-bg); }
.neutral { color: var(--zg-text-secondary); background: var(--zg-surface-muted); }
</style>
