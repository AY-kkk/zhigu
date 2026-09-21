<template>
  <div class="zhigu-page">
    <h1>我的研究</h1>
    <p class="lead">回看观点与证据，重新验证新的问题。</p>
    <ResearchHistoryList
      :items="items"
      :instruments="conversation.instruments"
      :cursor="cursor"
      :busy="busy"
      :error="error"
      :loaded="loaded"
      @more="load(false)"
      @retry="load(true)"
      @open="openRun"
      @start="startNew"
      @reresearch="reResearch"
      @delete="onDelete"
    />
  </div>
</template>
<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Modal, Toast } from '@kousum/semi-ui-vue'
import ResearchHistoryList from '../../components/research/ResearchHistoryList.vue'
import { listResearch } from '../../api/research.js'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { getConsumerPopupContainer } from '../../utils/popup.js'
import { ACTIVE_STATUSES, historyTitle, mapRequestError } from '../../utils/researchCopy.js'

const router = useRouter()
const conversation = useResearchConversation()
const items = ref([])
const cursor = ref('')
const busy = ref(false)
const loaded = ref(false)
const error = ref('')

async function load(reset) {
  busy.value = true
  try {
    const res = await listResearch({ limit: 20, cursor: reset ? '' : cursor.value })
    const page = res.data.items || []
    const seen = new Set(reset ? [] : items.value.map((item) => item.run_id))
    const next = (reset ? [] : items.value).concat(page.filter((item) => !seen.has(item.run_id)))
    items.value = next
    cursor.value = res.data.next_cursor || ''
    error.value = ''
    loaded.value = true
  } catch (e) {
    error.value = mapRequestError(e)
    if (!items.value.length) loaded.value = true
  } finally {
    busy.value = false
  }
}

function openRun(id) {
  conversation.loadRun(id)
  router.push(`/app/research/${id}`)
}
function startNew() {
  conversation.resetAll()
  router.push('/app/research/new')
}
function reResearch(id) {
  router.push({ path: '/app/research/new', query: { parent_run_id: id } })
}
function onDelete(row) {
  const title = historyTitle(row)
  const active = ACTIVE_STATUSES.includes(row.status)
  Modal.confirm({
    title: '从研究记录中移除？',
    content: active
      ? `将先停止「${title}」相关执行，再从研究记录移除并安排清理。这不是立即销毁全部副本。取消研究与删除是不同操作。`
      : `将「${title}」从研究记录移除并安排清理。这不是立即销毁全部副本。`,
    okText: '确认删除',
    cancelText: '返回',
    okType: 'danger',
    getPopupContainer: getConsumerPopupContainer,
    onOk: async () => {
      const result = await conversation.deleteRun(row.run_id)
      if (!result.ok) {
        Toast.error({ content: result.error || '删除失败，可重试' })
        return
      }
      Toast.info({ content: '删除处理中', duration: 2 })
      items.value = items.value.map((item) => item.run_id === row.run_id ? { ...item, deleting: true } : item)
      await load(true)
    }
  })
}

onMounted(() => {
  conversation.ensureInstruments()
  load(true)
})
</script>
<style scoped>
h1 { margin: 0 0 8px; font-size: 28px; line-height: 38px; font-weight: 600; }
.lead { margin: 0 0 24px; color: var(--zg-text-secondary); font-size: 16px; line-height: 28px; }
</style>
