<template>
  <section class="status" role="status">
    <p class="stage">{{ message }}</p>
    <p class="note">进度来自服务端阶段，不显示百分比或模拟日志。</p>
    <ol class="steps">
      <li v-for="step in steps" :key="step.key" :class="{ done: step.done, current: step.current }">
        <span class="mark" aria-hidden="true">{{ step.done ? '●' : step.current ? '◉' : '○' }}</span>
        {{ step.label }}
      </li>
    </ol>
    <p v-if="runError" class="err">{{ runError }}</p>
    <Button v-if="canCancel" type="danger" theme="light" @click="conversation.cancelCurrent">取消研究</Button>
  </section>
</template>
<script setup>
import { computed } from 'vue'
import { Button } from '@kousum/semi-ui-vue'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { STATUS_MESSAGES } from '../../utils/researchCopy.js'

const conversation = useResearchConversation()
const status = computed(() => conversation.runView?.status || '')
const message = computed(() => STATUS_MESSAGES[status.value] || '正在读取研究状态')
const runError = computed(() => conversation.runError)
const canCancel = computed(() => ['queued', 'researching', 'verifying', 'canceling'].includes(status.value) && status.value !== 'canceled')
const steps = computed(() => {
  const order = ['queued', 'researching', 'verifying']
  const current = status.value
  const currentIndex = order.indexOf(current)
  return [
    { key: 'queued', label: '排队中' },
    { key: 'researching', label: '调查中' },
    { key: 'verifying', label: '核对中' }
  ].map((step, index) => ({
    ...step,
    current: step.key === current,
    done: currentIndex > index || ['completed', 'incomplete', 'failed', 'canceled'].includes(current)
  })).filter((step) => step.done || step.current || currentIndex === -1 && step.key === 'queued')
})
</script>
<style scoped>
.status { padding: 4px 0 8px; max-width: 100%; }
.stage { margin: 0 0 4px; font-size: 16px; line-height: 24px; font-weight: 500; }
.note, .err { margin: 0 0 12px; font-size: 12px; line-height: 18px; color: var(--semi-color-text-1); }
.err { color: var(--zg-danger); }
.steps { list-style: none; padding: 0; margin: 0 0 16px; display: grid; gap: 8px; }
.steps li { color: var(--semi-color-text-1); font-size: 14px; }
.steps li.current { color: var(--zg-brand-text); font-weight: 500; }
.mark { margin-right: 8px; }
</style>
