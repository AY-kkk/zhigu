<script setup>
import { computed } from 'vue'

const props = defineProps({
  state: { type: Object, required: true },
})

const rows = computed(() => [
  ['部署环境', props.state.deploymentEnvironment],
  ['操作模式', props.state.operationMode],
  ['数据来源', props.state.dataProvenance],
  ['健康状态', props.state.health],
])

const tone = computed(() => {
  if (props.state.health === 'ready') return 'ready'
  if (props.state.health === 'degraded') return 'warning'
  return 'blocked'
})
</script>

<template>
  <section class="capability-status" aria-label="模块运行状态">
    <span class="system-status" :data-tone="tone">{{ state.health }}</span>
    <dl>
      <div v-for="[label, value] in rows" :key="label">
        <dt>{{ label }}</dt>
        <dd>{{ value }}</dd>
      </div>
    </dl>
    <p v-if="state.reason">{{ state.reason }}</p>
  </section>
</template>
