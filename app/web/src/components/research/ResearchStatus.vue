<template>
  <section class="status" aria-live="polite">
    <p class="stage">{{ message }}</p>
    <p v-if="disconnected" class="note">连接中断，显示上次状态</p>
    <p v-else-if="updatedText" class="note">最近更新 {{ updatedText }}</p>
    <ResearchStageRail :status="status" :report="report" />
    <p v-if="runError && !disconnected" class="err">{{ runError }}</p>
    <button
      v-if="canCancel"
      type="button"
      class="zg-btn zg-btn-danger"
      :disabled="status === 'canceling'"
      @click="conversation.cancelCurrent"
    >
      {{ status === 'canceling' ? '取消中' : '取消研究' }}
    </button>
  </section>
</template>
<script setup>
import { computed } from 'vue'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { CANCELABLE_STATUSES, STATUS_MESSAGES } from '../../utils/researchCopy.js'
import { formatDateTime } from '../../utils/researchCopy.js'
import ResearchStageRail from './ResearchStageRail.vue'

const conversation = useResearchConversation()
const status = computed(() => conversation.runView?.status || '')
const report = computed(() => conversation.runView?.report || null)
const message = computed(() => STATUS_MESSAGES[status.value] || '暂无法识别研究状态')
const runError = computed(() => conversation.runError)
const disconnected = computed(() => conversation.disconnected)
const canCancel = computed(() => CANCELABLE_STATUSES.includes(status.value) || status.value === 'canceling')
const updatedText = computed(() => formatDateTime(conversation.runView?.updated_at))
</script>
<style scoped>
.status { padding: 24px; background: var(--zg-surface); border: 1px solid var(--zg-line); border-radius: var(--zg-radius-card); }
.stage { margin: 0 0 4px; font-size: 18px; line-height: 28px; font-weight: 600; }
.note, .err { margin: 0 0 12px; font-size: 12px; line-height: 18px; color: var(--zg-text-secondary); }
.err { color: var(--zg-error-fg); }
@media (max-width: 767px) { .status { padding: 20px 16px; } }
</style>
