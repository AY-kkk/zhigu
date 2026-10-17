<template>
  <ConfigProvider :locale="zhCN" :getPopupContainer="getConsumerPopupContainer">
    <AppShell :mobile="isMobile">
      <template #side>
        <SideNav />
      </template>
      <template #header>
        <WorkspaceHeader
          :is-mobile="isMobile"
          :history-active="isHistory"
          :title="route.meta?.title || ''"
          @open-nav="navOpen = true"
          @delete-research="onDelete"
        />
      </template>
      <DataModeNotice v-if="!route.path.startsWith('/app/strategies')" />
      <router-view />
      <template #overlays>
        <SideSheet
          v-if="isMobile"
          class-name="zhigu-consumer zhigu-mobile-nav"
          title="知股"
          :visible="navOpen"
          placement="left"
          :width="260"
          :close-on-esc="true"
          :close-icon="navCloseIcon"
          :z-index="210"
          :motion="false"
          :get-popup-container="getConsumerPopupContainer"
          :on-cancel="() => navOpen = false"
        >
          <SideNav stacked @navigated="navOpen = false" />
        </SideSheet>
        <EvidenceDrawer
          :open="conversation.evidenceOpen"
          :evidence="conversation.evidence"
          :error="conversation.evidenceError"
          :busy="conversation.evidenceBusy"
          :relation="conversation.evidenceRelation"
          :index="evidenceIndex"
          @close="conversation.closeEvidence"
          @retry="conversation.retryEvidence"
        />
        <Teleport to="body">
          <div id="zhigu-overlay-root" class="zhigu-consumer zhigu-overlay-root"></div>
        </Teleport>
      </template>
    </AppShell>
  </ConfigProvider>
</template>
<script setup>
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ConfigProvider,
  Modal,
  SideSheet,
  Toast
} from '@kousum/semi-ui-vue'
import { IconClose } from '@kousum/semi-icons-vue'
import zhCN from '@kousum/semi-ui-vue/dist/locale/source/zh_CN.js'
import AppShell from '../../components/workspace/AppShell.vue'
import SideNav from '../../components/workspace/SideNav.vue'
import WorkspaceHeader from '../../components/workspace/WorkspaceHeader.vue'
import EvidenceDrawer from '../../components/research/EvidenceDrawer.vue'
import DataModeNotice from '../../components/research/DataModeNotice.vue'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { getConsumerPopupContainer } from '../../utils/popup.js'
import { ACTIVE_STATUSES, historyTitle } from '../../utils/researchCopy.js'
import { evidenceIndexMap } from '../../utils/researchViewModel.js'
import '@kousum/semi-ui-vue/dist/_base/base.css'
import '../../styles/consumer-semi.css'

const route = useRoute()
const router = useRouter()
const conversation = useResearchConversation()
const navOpen = ref(false)
const navCloseIcon = h(IconClose, { 'aria-label': '关闭导航' })
const viewport = ref(typeof window === 'undefined' ? 1440 : window.innerWidth)
const isMobile = computed(() => viewport.value < 768)
const isHistory = computed(() => route.path.startsWith('/app/history'))
const evidenceIndex = computed(() => {
  const map = evidenceIndexMap(conversation.runView?.report)
  return map[conversation.requestedEvidenceId] || 0
})

function updateViewport() {
  viewport.value = window.innerWidth
}
onMounted(() => {
  updateViewport()
  window.addEventListener('resize', updateViewport)
  conversation.ensureInstruments()
  Toast.config({ getPopupContainer: getConsumerPopupContainer, zIndex: 400 })
})
onBeforeUnmount(() => window.removeEventListener('resize', updateViewport))

function onDelete() {
  const id = conversation.currentRunId
  const title = historyTitle({
    instrument_id: conversation.runView?.instrument_id || conversation.instrumentId || id
  })
  const active = ACTIVE_STATUSES.includes(conversation.runView?.status)
  Modal.confirm({
    title: '从研究记录中移除？',
    content: active
      ? `将停止「${title}」的执行，并从研究记录移除、安排清理。这不是立即销毁全部副本。取消研究与删除是不同操作。`
      : `将「${title}」从研究记录移除并安排清理。这不是立即销毁全部副本。取消研究与删除是不同操作。`,
    okText: '确认删除',
    cancelText: '返回',
    okType: 'danger',
    okButtonProps: { 'aria-label': '确认删除' },
    cancelButtonProps: { 'aria-label': '返回' },
    zIndex: 300,
    getPopupContainer: getConsumerPopupContainer,
    onOk: async () => {
      const result = await conversation.deleteRun(id)
      if (result.ok) router.push('/app/history')
    }
  })
}
</script>
