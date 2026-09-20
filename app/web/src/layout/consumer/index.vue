<template>
  <ConfigProvider :locale="zhCN" :getPopupContainer="getConsumerPopupContainer">
    <div class="zhigu-consumer zhigu-app-shell">
      <div class="zhigu-shell-inner">
        <Layout>
          <LayoutSider v-if="!isMobile" class="zhigu-sider" :width="88">
            <SideNav />
          </LayoutSider>
          <Layout class="zhigu-main">
            <LayoutHeader class="zhigu-header">
              <WorkspaceHeader
                :is-mobile="isMobile"
                @open-nav="navOpen = true"
                @open-history="openHistory"
                @delete-research="onDelete"
              />
            </LayoutHeader>
            <LayoutContent class="zhigu-content" :class="{ 'is-scrollable': !isResearch }">
              <FixtureBanner />
              <router-view />
            </LayoutContent>
          </Layout>
        </Layout>
      </div>
      <SideSheet
        v-if="isMobile"
        class="zhigu-consumer"
        title="知股"
        :visible="navOpen"
        placement="left"
        :width="220"
        :closeOnEsc="true"
        :getPopupContainer="getConsumerPopupContainer"
        :onCancel="() => navOpen = false"
      >
        <SideNav @navigated="navOpen = false" />
      </SideSheet>
      <HistoryDrawer
        :open="conversation.historyOpen"
        :items="history.items"
        :cursor="history.cursor"
        :busy="history.busy"
        :error="history.error"
        @close="conversation.closeHistory"
        @more="loadHistory(false)"
        @open="onOpenHistoryItem"
        @start="onStartFromHistory"
      />
      <EvidenceDrawer
        :open="conversation.evidenceOpen"
        :evidence="conversation.evidence"
        :error="conversation.evidenceError"
        @close="conversation.closeEvidence"
        @retry="retryEvidence"
      />
      <div id="zhigu-overlay-root" class="zhigu-consumer zhigu-overlay-root"></div>
    </div>
  </ConfigProvider>
</template>
<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ConfigProvider,
  Layout,
  LayoutContent,
  LayoutHeader,
  LayoutSider,
  Modal,
  SideSheet,
  Toast
} from '@kousum/semi-ui-vue'
import zhCN from '@kousum/semi-ui-vue/dist/locale/source/zh_CN.js'
import SideNav from '../../components/workspace/SideNav.vue'
import WorkspaceHeader from '../../components/workspace/WorkspaceHeader.vue'
import HistoryDrawer from '../../components/workspace/HistoryDrawer.vue'
import EvidenceDrawer from '../../components/research/EvidenceDrawer.vue'
import FixtureBanner from '../../components/research/FixtureBanner.vue'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { listResearch } from '../../api/research.js'
import { getConsumerPopupContainer } from '../../utils/popup.js'
import { historyTitle, mapRequestError } from '../../utils/researchCopy.js'
import '../../styles/consumer-semi.css'

const route = useRoute()
const router = useRouter()
const conversation = useResearchConversation()
const navOpen = ref(false)
const isMobile = ref(false)
const history = reactive({ items: [], cursor: '', busy: false, error: '' })
const isResearch = computed(() => route.path.startsWith('/app/research'))

function updateViewport() {
  isMobile.value = window.innerWidth < 768
}
onMounted(() => {
  updateViewport()
  window.addEventListener('resize', updateViewport)
  Toast.config({ getPopupContainer: getConsumerPopupContainer })
})
onBeforeUnmount(() => window.removeEventListener('resize', updateViewport))

watch(() => conversation.historyOpen, (open) => {
  if (open) loadHistory(true)
})

async function loadHistory(reset) {
  history.busy = true
  try {
    const res = await listResearch({ limit: 20, cursor: reset ? '' : history.cursor })
    const page = res.data.items || []
    history.items = reset ? page : history.items.concat(page)
    history.cursor = res.data.next_cursor || ''
    history.error = ''
  } catch (error) {
    history.error = mapRequestError(error)
  } finally {
    history.busy = false
  }
}

function openHistory() {
  conversation.openHistory()
}

function onOpenHistoryItem(id) {
  conversation.loadRun(id)
}

function onStartFromHistory() {
  conversation.resetAll()
}

function retryEvidence() {
  const id = conversation.evidence?.evidence_id
  if (id) conversation.openEvidence(id, conversation.evidenceSourceEl)
}

function onDelete() {
  const title = historyTitle({ instrument_id: conversation.runView?.instrument_id || conversation.instrumentId || conversation.currentRunId })
  Modal.confirm({
    title: '删除这项研究？',
    content: `将删除「${title}」及其报告，此操作不可恢复。取消、删除与重新研究是相互独立的操作。`,
    okText: '删除',
    cancelText: '返回',
    okType: 'danger',
    getPopupContainer: getConsumerPopupContainer,
    onOk: async () => {
      const ok = await conversation.deleteCurrent()
      if (ok) router.push('/app/profile')
    }
  })
}
</script>
