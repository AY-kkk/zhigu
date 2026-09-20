<template>
  <div class="side-wrap">
    <router-link class="zhigu-brand" to="/app/research/new" aria-label="知股">
      <span class="zhigu-brand-mark" aria-hidden="true">知</span>
      <span class="zhigu-brand-name">知股</span>
    </router-link>
    <Nav
      class="zhigu-side-nav"
      mode="vertical"
      :isCollapsed="false"
      :defaultIsCollapsed="false"
      :selectedKeys="[selected]"
      :items="items"
      :onSelect="onSelect"
      :onClick="onNavClick"
      :renderWrapper="wrapItem"
    />
  </div>
</template>
<script setup>
import { computed, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Nav } from '@kousum/semi-ui-vue'
import { IconComment, IconHistogram, IconUser } from '@kousum/semi-icons-vue'
import { useResearchConversation } from '../../stores/researchConversation.js'

const emit = defineEmits(['navigated'])
const route = useRoute()
const router = useRouter()
const conversation = useResearchConversation()

const selected = computed(() => {
  if (route.path.startsWith('/app/profile') || route.path.startsWith('/app/history')) return 'profile'
  if (route.path.startsWith('/app/strategies')) return 'strategies'
  return 'research'
})

const items = [
  { itemKey: 'profile', text: '个人界面', icon: h(IconUser, { size: 'large' }) },
  { itemKey: 'research', text: '投研观点', icon: h(IconComment, { size: 'large' }) },
  { itemKey: 'strategies', text: '交易策略', icon: h(IconHistogram, { size: 'large' }) }
]

const paths = {
  profile: '/app/profile',
  research: '/app/research/new',
  strategies: '/app/strategies'
}

function wrapItem({ itemElement, props }) {
  const current = props.itemKey === selected.value
  return h('div', { 'aria-current': current ? 'page' : undefined }, [itemElement])
}

function onNavClick(data) {
  onSelect(data)
}

function onSelect(data) {
  const key = data?.itemKey
  if (!key || !paths[key]) return
  if (key === 'research') {
    const runId = conversation.currentRunId
    const target = runId ? `/app/research/${runId}` : '/app/research/new'
    if (route.path !== target) router.push(target)
    emit('navigated')
    return
  }
  router.push(paths[key])
  emit('navigated')
}
</script>
<style scoped>
.side-wrap {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--zg-sidebar-bg);
}
</style>
