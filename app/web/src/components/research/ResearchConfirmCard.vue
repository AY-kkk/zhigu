<template>
  <Card class="confirm-card" :bordered="true" shadows="never" title="确认研究范围">
    <p class="lead">选择证券和研究期限后，才会创建研究任务。</p>
    <div class="field">
      <label for="instrument">证券</label>
      <Select
        id="instrument"
        :value="conversation.instrumentId"
        placeholder="选择证券"
        style="width: 100%"
        :getPopupContainer="getConsumerPopupContainer"
        :onChange="conversation.setInstrument"
      >
        <SelectOption v-for="item in candidates" :key="item.instrument_id" :value="item.instrument_id">
          {{ item.name || item.instrument_id }}
        </SelectOption>
      </Select>
    </div>
    <div class="field">
      <label for="horizon">研究期限</label>
      <Input id="horizon" :value="conversation.horizon" placeholder="例如：未来一年" :onChange="conversation.setHorizon" />
    </div>
    <p v-if="asOf" class="meta">数据截止时间将在确认时冻结。</p>
    <div v-if="items.length" class="claims">
      <p>拆解主张（最多 6 条）</p>
      <ul>
        <li v-for="item in items" :key="item.claim_id">
          <Tag>{{ claimType(item.claim_type) }}</Tag>
          <span>{{ item.text }}</span>
        </li>
      </ul>
    </div>
    <p v-if="missing" class="err">{{ missing }}</p>
    <p v-if="conversation.confirmError" class="err">{{ conversation.confirmError }}</p>
    <router-link v-if="conversation.activeConflictId" class="back" :to="`/app/research/${conversation.activeConflictId}`">返回进行中的研究</router-link>
    <div class="actions">
      <Button type="tertiary" @click="editClaim">修改原观点</Button>
      <Button type="primary" theme="solid" :loading="conversation.confirmBusy" :disabled="!conversation.canStart" @click="onConfirm">确认并开始研究</Button>
    </div>
  </Card>
</template>
<script setup>
import { computed } from 'vue'
import { Button, Card, Input, Select, SelectOption, Tag } from '@kousum/semi-ui-vue'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { CLAIM_TYPE_LABELS } from '../../utils/researchCopy.js'
import { getConsumerPopupContainer } from '../../utils/popup.js'

const emit = defineEmits(['confirmed', 'edit-claim'])
const conversation = useResearchConversation()
const candidates = computed(() => conversation.draft?.candidates || [])
const items = computed(() => (conversation.draft?.items || []).slice(0, 6))
const asOf = computed(() => true)
const missing = computed(() => {
  if (!conversation.instrumentId) return '请选择有效证券后再确认。'
  if (!conversation.horizon.trim()) return '请填写研究期限后再确认。'
  return ''
})
function claimType(type) { return CLAIM_TYPE_LABELS[type] || type || '主张' }
function editClaim() { emit('edit-claim') }
async function onConfirm() {
  const id = await conversation.startResearch()
  if (id) emit('confirmed', id)
}
</script>
<style scoped>
.confirm-card { max-width: 100%; border-radius: 12px; }
.confirm-card :deep(.semi-card-body) { padding: 20px; }
.lead, .meta { color: var(--semi-color-text-1); font-size: 14px; line-height: 22px; }
.field { display: grid; gap: 8px; margin: 16px 0; }
.field label { font-size: 14px; font-weight: 500; }
.claims ul { list-style: none; padding: 0; margin: 8px 0 0; display: grid; gap: 8px; }
.claims li { display: flex; gap: 8px; align-items: flex-start; }
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
.err { color: var(--zg-danger); }
.back { color: var(--zg-brand-text); }
</style>
