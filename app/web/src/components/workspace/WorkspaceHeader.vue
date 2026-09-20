<template>
  <div class="header">
    <Button
      v-if="isMobile"
      class="menu-btn"
      type="tertiary"
      theme="borderless"
      :icon="h(IconMenu)"
      aria-label="打开导航"
      @click="$emit('open-nav')"
    />
    <p class="zhigu-header-title title">{{ title }}</p>
    <div class="actions">
      <Button type="tertiary" theme="borderless" :icon="h(IconHistory)" @click="$emit('open-history')">历史记录</Button>
      <Button v-if="showNew" type="primary" theme="solid" :icon="h(IconPlus)" @click="onNew">新研究</Button>
      <Dropdown trigger="click" position="bottomRight" :getPopupContainer="getConsumerPopupContainer" :menu="menu">
        <Button type="tertiary" theme="borderless" :icon="h(IconMore)" aria-label="更多操作" />
      </Dropdown>
    </div>
  </div>
</template>
<script setup>
import { computed, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Button, Dropdown } from '@kousum/semi-ui-vue'
import { IconHistory, IconMenu, IconMore, IconPlus } from '@kousum/semi-icons-vue'
import { useSession } from '../../stores/session.js'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { getConsumerPopupContainer } from '../../utils/popup.js'
import { historyTitle } from '../../utils/researchCopy.js'

const emit = defineEmits(['open-nav', 'open-history', 'delete-research'])
defineProps({
  isMobile: { type: Boolean, default: false }
})
const route = useRoute()
const router = useRouter()
const session = useSession()
const conversation = useResearchConversation()

const title = computed(() => {
  if (route.path.startsWith('/app/profile')) return '个人界面'
  if (route.path.startsWith('/app/strategies')) return '交易策略'
  if (route.path.startsWith('/app/research/') && route.params.id) {
    return historyTitle({ instrument_id: conversation.runView?.instrument_id || conversation.instrumentId })
  }
  return '投研观点'
})

const showNew = computed(() => {
  if (!route.path.startsWith('/app/research')) return true
  if (route.path === '/app/research/new' && !conversation.chats.length && !conversation.draftText) return false
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
  margin: 0 !important;
  font-size: 16px !important;
  line-height: 24px !important;
  font-weight: 600 !important;
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
</style>
