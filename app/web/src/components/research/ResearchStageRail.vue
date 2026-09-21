<template>
  <ol class="rail" aria-label="研究阶段">
    <li v-for="step in steps" :key="step.key" :class="step.state">
      <span class="dot" aria-hidden="true">
        <ZhiguIcon v-if="step.state === 'done'" name="check" :size="20" />
        <ZhiguIcon v-else-if="step.key === 'queued'" name="queue" :size="20" />
        <ZhiguIcon v-else-if="step.key === 'researching'" name="search" :size="20" />
        <ZhiguIcon v-else-if="step.key === 'verifying'" name="verify" :size="20" />
        <ZhiguIcon v-else name="report" :size="20" />
      </span>
      <span class="label">{{ step.label }}</span>
    </li>
  </ol>
</template>
<script setup>
import { computed } from 'vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'
import { stageRailModel } from '../../utils/researchViewModel.js'

const props = defineProps({
  status: { type: String, default: '' },
  report: { type: Object, default: null }
})
const steps = computed(() => stageRailModel(props.status, props.report))
</script>
<style scoped>
.rail {
  list-style: none;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 0;
  padding: 0;
  margin: 28px 0;
}
li {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  color: var(--zg-text-secondary);
  font-size: 12px;
  line-height: 18px;
}
li:not(:last-child)::after { content: ''; position: absolute; top: 24px; left: calc(50% + 29px); right: calc(-50% + 29px); height: 1px; background: var(--zg-line); }
.dot {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  border: 1px solid var(--zg-line);
  display: grid;
  place-items: center;
  background: var(--zg-surface);
}
.current { color: var(--zg-action); font-weight: 600; }
.current .dot { border-color: var(--zg-action); color: var(--zg-action); background: var(--zg-support-bg); box-shadow: 0 0 0 5px var(--zg-paper); }
.done { color: var(--zg-ink); }
.done .dot { border-color: var(--zg-ink); color: var(--zg-ink); }
.partial .dot { border-style: dashed; }
@media (max-width: 767px) {
  .dot { width: 40px; height: 40px; }
  li:not(:last-child)::after { top: 20px; left: calc(50% + 24px); right: calc(-50% + 24px); }
}
</style>
