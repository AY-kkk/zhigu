<template>
  <Tooltip v-if="valid" :content="title || `查看证据 ${number}`">
    <button class="ref" type="button" :aria-label="ariaName" @click="onClick">[{{ number }}]</button>
  </Tooltip>
  <span v-else class="plain" :title="'引用不可用'">[{{ raw }}]</span>
</template>
<script setup>
import { computed } from 'vue'
import { Tooltip } from '@kousum/semi-ui-vue'
const props = defineProps({
  evidenceId: { type: String, required: true },
  allowedIds: { type: Array, default: () => [] },
  indexMap: { type: Object, default: () => ({}) },
  title: { type: String, default: '' }
})
const emit = defineEmits(['open'])
const valid = computed(() => (props.allowedIds || []).includes(props.evidenceId))
const number = computed(() => props.indexMap[props.evidenceId] || '')
const raw = computed(() => props.indexMap[props.evidenceId] || props.evidenceId)
const ariaName = computed(() => (
  props.title ? `查看证据 ${number.value}：${props.title}` : `查看证据 ${number.value}`
))
function onClick(event) {
  if (!valid.value) return
  emit('open', props.evidenceId, event.currentTarget)
}
</script>
<style scoped>
.ref {
  margin-left: 4px;
  padding: 0 4px;
  border: 0;
  background: transparent;
  color: var(--zg-action);
  cursor: pointer;
  font: inherit;
  min-width: 32px;
  min-height: 32px;
}
.plain { color: var(--zg-text-secondary); }
</style>
