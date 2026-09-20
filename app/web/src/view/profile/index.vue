<template>
  <div class="zhigu-page">
    <section class="account">
      <Avatar :alt="session.username" size="medium">{{ initial }}</Avatar>
      <div>
        <TypographyTitle heading="4">{{ session.username || '受邀用户' }}</TypographyTitle>
        <p class="meta">当前登录账户。不提供风险测评、资产绑定或付费中心。</p>
        <Button type="tertiary" :icon="h(IconExit)" @click="onLogout">退出登录</Button>
      </div>
    </section>
    <TypographyTitle heading="5">我的研究</TypographyTitle>
    <ResearchHistoryList
      :items="items"
      :cursor="cursor"
      :busy="busy"
      :error="error"
      @more="load(false)"
      @open="openRun"
      @start="startNew"
    />
  </div>
</template>
<script setup>
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Avatar, Button, TypographyTitle } from '@kousum/semi-ui-vue'
import { IconExit } from '@kousum/semi-icons-vue'
import ResearchHistoryList from '../../components/research/ResearchHistoryList.vue'
import { listResearch } from '../../api/research.js'
import { useSession } from '../../stores/session.js'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { mapRequestError } from '../../utils/researchCopy.js'

const router = useRouter()
const session = useSession()
const conversation = useResearchConversation()
const items = ref([])
const cursor = ref('')
const busy = ref(false)
const error = ref('')
const initial = computed(() => (session.username || '知').slice(0, 1))

async function load(reset) {
  busy.value = true
  try {
    const res = await listResearch({ limit: 20, cursor: reset ? '' : cursor.value })
    const page = res.data.items || []
    items.value = reset ? page : items.value.concat(page)
    cursor.value = res.data.next_cursor || ''
    error.value = ''
  } catch (e) {
    error.value = mapRequestError(e)
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
function onLogout() {
  conversation.resetAll()
  session.logout()
  router.push('/login')
}
onMounted(() => load(true))
</script>
<style scoped>
.account { display: flex; gap: 16px; align-items: flex-start; margin-bottom: 32px; }
.meta { color: var(--semi-color-text-1); margin: 8px 0; }
</style>
