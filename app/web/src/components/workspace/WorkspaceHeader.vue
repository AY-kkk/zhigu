<template>
  <div class="header">
    <button
      v-if="isMobile"
      type="button"
      class="zg-btn zg-btn-ghost menu-btn"
      aria-label="打开导航"
      @click="$emit('open-nav')"
    >
      <ZhiguIcon name="menu" :size="20" />
    </button>
    <p class="zhigu-header-title title">{{ title }}</p>
    <div class="actions">
      <button
        type="button"
        class="zg-btn zg-btn-ghost history-btn"
        :class="{ 'is-current': historyActive }"
        :aria-current="historyActive ? 'page' : undefined"
        :aria-label="isMobile ? '历史记录' : undefined"
        @click="goHistory"
      >
        <ZhiguIcon v-if="isMobile" name="history" :size="20" />
        <span v-else>历史记录</span>
      </button>
      <button v-if="showNew" type="button" class="zg-btn zg-btn-primary" @click="onNew">
        <ZhiguIcon name="plus" :size="16" />
        新研究
      </button>
      <Dropdown trigger="click" position="bottomRight" :getPopupContainer="getConsumerPopupContainer" :menu="menu">
        <button type="button" class="zg-btn zg-btn-ghost" aria-label="更多操作">
          <ZhiguIcon name="more" :size="20" />
        </button>
      </Dropdown>
    </div>
  </div>
</template>
<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Dropdown } from '@kousum/semi-ui-vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'
import { useSession } from '../../stores/session.js'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { getConsumerPopupContainer } from '../../utils/popup.js'
import { historyTitle } from '../../utils/researchCopy.js'

const emit = defineEmits(['open-nav', 'delete-research'])
const props = defineProps({
  isMobile: { type: Boolean, default: false },
  historyActive: { type: Boolean, default: false }
})
const route = useRoute()
const router = useRouter()
const session = useSession()
const conversation = useResearchConversation()

const title = computed(() => {
  if (route.path.startsWith('/app/history')) return '我的研究'
  if (route.path.startsWith('/app/profile')) return '个人界面'
  if (route.path.startsWith('/app/strategies')) return '交易策略'
  if (route.path.startsWith('/app/research/') && route.params.id) {
    const id = conversation.runView?.instrument_id || conversation.instrumentId
    const match = conversation.instruments.find((row) => row.instrument_id === id)
    if (props.isMobile) return match?.name || id || '观点研究'
    return historyTitle({ instrument_id: id, instrument_name: match?.name })
  }
  return '投研观点'
})

const showNew = computed(() => {
  if (route.path.startsWith('/app/strategies')) return false
  if (route.path === '/app/research/new' && !conversation.draftText && !conversation.userMessage) return false
  return true
})

const canDelete = computed(() => route.path.startsWith('/app/research/') && !!route.params.id)
const menu = computed(() => {
  const items = []
  if (canDelete.value) {
    items.push({ node: 'item', name: '删除研究', type: 'danger', onClick: () => emit('delete-research') })
  }
  items.push({ node: 'item', name: '退出登录', onClick: onLogout })
  return items
})

function goHistory() {
  router.push('/app/history')
}

function onNew() {
  conversation.resetAll()
  router.push('/app/research/new')
}

function onLogout() {
  conversation.resetAll()
  session.logout()
  router.push('/login')
}
</script>
<style scoped>
.header {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  min-width: 0;
}
.title {
  margin: 0;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
}
.menu-btn { flex: none; }
.history-btn { flex: none; }
.is-current {
  font-weight: 600;
  color: var(--zg-action);
}
@media (max-width: 767px) {
  .title { font-size: 14px; }
}
</style>
