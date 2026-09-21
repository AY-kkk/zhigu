<template>
  <Card class="confirm-card" :bordered="true" shadows="never" title="确认研究范围">
    <p class="lead">选择标的公司和研究期限后，才会创建研究任务。确认后将冻结本次研究的数据截止时间。</p>
    <div class="field">
      <label for="instrument">标的公司</label>
      <Select
        id="instrument"
        :value="conversation.instrumentId"
        placeholder="搜索 A 股或港股代码、名称"
        filter
        remote
        style="width: 100%"
        :getPopupContainer="getConsumerPopupContainer"
        :zIndex="220"
        :onChange="conversation.setInstrument"
        :onSearch="onSearch"
      >
        <SelectOption v-for="item in candidates" :key="item.instrument_id" :value="item.instrument_id">
          {{ instrumentLabel(item) }}
        </SelectOption>
      </Select>
    </div>
    <div class="field">
      <label for="horizon">研究期限</label>
      <Input id="horizon" :value="conversation.horizon" placeholder="请确认研究期限" :onChange="conversation.setHorizon" />
    </div>
    <div v-if="items.length" class="claims">
      <p>关键主张（最多 6 条）</p>
      <ul>
        <li v-for="item in items" :key="item.claim_id">
          <EvidenceTag :kind="item.claim_type" />
          <span>{{ item.text }}</span>
        </li>
      </ul>
    </div>
    <p v-if="missing" class="err">{{ missing }}</p>
    <p v-if="conversation.confirmError" class="err">{{ conversation.confirmError }}</p>
    <router-link
      v-if="conversation.activeConflictId"
      class="back"
      :to="`/app/research/${conversation.activeConflictId}`"
    >返回进行中的研究</router-link>
    <div class="actions">
      <button type="button" class="zg-btn zg-btn-secondary" @click="editClaim">修改原观点</button>
      <button
        type="button"
        class="zg-btn zg-btn-primary"
        :disabled="!conversation.canStart"
        @click="onConfirm"
      >
        {{ conversation.confirmBusy ? '正在创建研究' : '确认并开始研究' }}
        <ZhiguIcon v-if="!conversation.confirmBusy" name="arrow" :size="16" />
      </button>
    </div>
  </Card>
</template>
<script setup>
import { computed, ref } from 'vue'
import { Card, Input, Select, SelectOption } from '@kousum/semi-ui-vue'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { getConsumerPopupContainer } from '../../utils/popup.js'
import { instrumentLabel } from '../../utils/researchViewModel.js'
import EvidenceTag from './EvidenceTag.vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'

const emit = defineEmits(['confirmed', 'edit-claim'])
const conversation = useResearchConversation()
const hits = ref([])
let searchTimer = 0
const candidates = computed(() => {
  const map = new Map()
  for (const row of [...(conversation.draft?.candidates || []), ...hits.value, ...conversation.instruments]) {
    if (row?.instrument_id) map.set(row.instrument_id, row)
  }
  return [...map.values()]
})
function onSearch(q) {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(async () => {
    hits.value = await conversation.searchInstruments(q)
  }, 200)
}
const items = computed(() => (conversation.draft?.items || []).slice(0, 6))
const missing = computed(() => {
  if (!conversation.instrumentId) return '请搜索并选择覆盖目录中的证券后再确认。'
  if (!conversation.horizon.trim()) return '请填写起止日期（YYYY-MM-DD/YYYY-MM-DD）后再确认。'
  return ''
})
function editClaim() {
  emit('edit-claim')
  conversation.requestComposerFocus()
}
async function onConfirm() {
  const id = await conversation.startResearch()
  if (id) emit('confirmed', id)
}
</script>
<style scoped>
.confirm-card { max-width: 100%; border-radius: var(--zg-radius-card); background: var(--zg-surface); }
.confirm-card :deep(.semi-card-body) { padding: 24px; }
.lead { color: var(--zg-text-secondary); font-size: 14px; line-height: 22px; margin: 0 0 8px; }
.field { display: grid; gap: 8px; margin: 16px 0; }
.field label { font-size: 14px; font-weight: 500; }
.claims ul { list-style: none; padding: 0; margin: 8px 0 0; display: grid; gap: 8px; }
.claims li { display: flex; gap: 8px; align-items: flex-start; }
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; flex-wrap: wrap; }
.err { color: var(--zg-error-fg); }
.back { color: var(--zg-action); }
@media (max-width: 767px) {
  .actions { flex-direction: column-reverse; }
  .actions .zg-btn-primary { width: 100%; }
}
</style>
