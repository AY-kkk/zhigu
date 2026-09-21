<template>
  <div class="notice" role="status" data-testid="fixture-banner"><ZhiguIcon name="info" :size="14" /><span>{{ text }}</span></div>
</template>
<script setup>
import { computed } from 'vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { STAGE_A_PUBLIC_MODE } from '../../config/publicMode.js'
import { modeNotice } from '../../utils/researchCopy.js'

const conversation = useResearchConversation()
const text = computed(() => {
  const mode = conversation.displayMode || STAGE_A_PUBLIC_MODE
  return modeNotice(mode)
})
</script>
<style scoped>
.notice {
  flex: none;
  padding: 8px var(--zg-page-pad);
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--zg-paper);
  color: var(--zg-text-secondary);
  font-size: 12px;
  line-height: 18px;
  border-bottom: 1px solid var(--zg-line);
}
</style>
