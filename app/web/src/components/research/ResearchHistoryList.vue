<template>
  <div>
    <div v-if="initialLoading" class="zg-skeleton" aria-busy="true">
      <div class="zg-skeleton-line" v-for="n in 4" :key="n" />
    </div>
    <EmptyResearchState
      v-else-if="!items.length && !error"
      title="还没有研究记录"
      description="开始验证你的投资观点，建立属于自己的研究记录。"
    >
      <button type="button" class="zg-btn zg-btn-primary" @click="$emit('start')">开始第一条研究</button>
    </EmptyResearchState>
    <ul v-else class="list">
      <li v-for="row in rows" :key="row.run_id" class="row">
        <div class="main">
          <p class="title">{{ row.title }}</p>
          <p class="meta">
            {{ row.createdText }} · {{ row.statusText }} · {{ row.modeText }}
            <span v-if="row.deleting"> · 删除处理中</span>
          </p>
        </div>
        <div class="ops">
          <button type="button" class="zg-btn zg-btn-secondary" @click="$emit('open', row.run_id)">打开</button>
          <Dropdown trigger="click" position="bottomRight" :getPopupContainer="getConsumerPopupContainer" :menu="menuFor(row)">
            <button type="button" class="zg-btn zg-btn-ghost" :aria-label="`更多操作 ${row.title}`">
              <ZhiguIcon name="more" :size="18" />
            </button>
          </Dropdown>
        </div>
      </li>
    </ul>
    <button v-if="cursor" type="button" class="zg-btn zg-btn-secondary" :disabled="busy" @click="$emit('more')">
      {{ busy ? '加载中' : '加载更多' }}
    </button>
    <ResearchErrorState v-if="error" title="无法加载研究记录" :message="error" retry-label="重试" @retry="$emit('retry')" />
  </div>
</template>
<script setup>
import { computed } from 'vue'
import { Dropdown } from '@kousum/semi-ui-vue'
import EmptyResearchState from './EmptyResearchState.vue'
import ResearchErrorState from './ResearchErrorState.vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'
import { getConsumerPopupContainer } from '../../utils/popup.js'
import { toHistoryRowModel } from '../../utils/researchViewModel.js'

const props = defineProps({
  items: { type: Array, default: () => [] },
  instruments: { type: Array, default: () => [] },
  cursor: { type: String, default: '' },
  busy: { type: Boolean, default: false },
  error: { type: String, default: '' },
  loaded: { type: Boolean, default: false }
})
const emit = defineEmits(['more', 'open', 'start', 'retry', 'reresearch', 'delete'])
const initialLoading = computed(() => props.busy && !props.loaded && !props.items.length)
const rows = computed(() => props.items.map((item) => toHistoryRowModel(item, props.instruments)))
function menuFor(row) {
  return [
    { node: 'item', name: '重新研究', onClick: () => emit('reresearch', row.run_id) },
    { node: 'item', name: '删除', type: 'danger', onClick: () => emit('delete', row) }
  ]
}
</script>
<style scoped>
.list { list-style: none; padding: 0; margin: 0 0 16px; }
.row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 0;
  border-bottom: 1px solid var(--zg-line);
}
.main { min-width: 0; flex: 1; }
.title { margin: 0; font-size: 16px; line-height: 24px; overflow-wrap: anywhere; }
.meta { margin: 4px 0 0; color: var(--zg-text-secondary); font-size: 12px; line-height: 18px; }
.ops { display: flex; gap: 4px; flex: none; }
@media (max-width: 767px) {
  .row { flex-wrap: wrap; }
  .ops { width: 100%; justify-content: flex-end; }
}
</style>
