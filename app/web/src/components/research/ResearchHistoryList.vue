<template>
  <div>
    <Empty v-if="!items.length && !busy && !error" description="还没有研究记录">
      <Button type="primary" theme="solid" @click="$emit('start')">开始第一条研究</Button>
    </Empty>
    <List v-else :dataSource="items" :split="true" :renderItem="renderItem" class="list" />
    <Button v-if="cursor" :loading="busy" @click="$emit('more')">加载更多</Button>
    <p v-if="error" class="err">{{ error }}</p>
  </div>
</template>
<script setup>
import { h } from 'vue'
import { Button, Empty, List, ListItem, Tag } from '@kousum/semi-ui-vue'
import { formatDateTime, historyTitle, modeLabel, statusLabel } from '../../utils/researchCopy.js'

const props = defineProps({
  items: { type: Array, default: () => [] },
  cursor: { type: String, default: '' },
  busy: { type: Boolean, default: false },
  error: { type: String, default: '' }
})
const emit = defineEmits(['more', 'open', 'start'])

function renderItem(item) {
  return h(ListItem, { class: 'history-row' }, {
    default: () => [
      h('div', { class: 'history-main' }, [
        h('p', { class: 'history-title' }, historyTitle(item)),
        h('p', { class: 'history-meta' }, [
          `${formatDateTime(item.created_at)} · ${statusLabel(item.status)} `,
          item.mode ? h(Tag, { size: 'small' }, { default: () => modeLabel(item.mode) }) : null
        ])
      ]),
      h(Button, { type: 'tertiary', onClick: () => emit('open', item.run_id) }, { default: () => '打开研究' })
    ]
  })
}
</script>
<style>
.history-row { display: flex; align-items: center; gap: 12px; }
.history-main { min-width: 0; flex: 1; }
.history-title { margin: 0; font-size: 16px; line-height: 24px; overflow-wrap: anywhere; }
.history-meta { margin: 4px 0 0; color: var(--semi-color-text-1, #716b70); font-size: 12px; display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
</style>
<style scoped>
.list { margin-bottom: 16px; }
.err { color: var(--zg-danger); }
</style>
