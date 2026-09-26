<template>
  <div class="side-wrap" :class="{ stacked }">
    <router-link v-if="!stacked" class="zhigu-brand" :to="brandTo" :aria-label="brandLabel">
      <ZhiguLogo variant="stacked" />
    </router-link>
    <nav class="zhigu-side-nav" :aria-label="navLabel">
      <button
        v-for="item in navItems"
        :key="item.key"
        type="button"
        class="zhigu-nav-item"
        :class="{ 'is-active': selected === item.key }"
        :aria-current="selected === item.key ? 'page' : undefined"
        :title="item.title"
        @click="go(item)"
      >
        <ZhiguIcon :name="item.icon" :size="24" />
        <span>{{ item.text }}</span>
      </button>
    </nav>
    <a
      v-if="intelRetryVisible"
      class="intel-retry-link"
      href="/app/intel"
      target="_blank"
      rel="noopener noreferrer"
      @click="emit('navigated')"
    >重试打开事件情报</a>
  </div>
</template>
<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ZhiguLogo from '../brand/ZhiguLogo.vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { openIntelWindow } from '../../utils/openModuleWindow.js'

const BASE_CONSUMER_ITEMS = [
  { key: 'profile', text: '个人界面', icon: 'account' },
  { key: 'research', text: '投研观点', icon: 'research' },
  { key: 'strategies', text: '交易策略', icon: 'strategies' }
]
const CONSUMER_ITEMS = import.meta.env.VITE_INTEL_ENABLED === 'true'
  ? [...BASE_CONSUMER_ITEMS, { key: 'intel', text: '事件情报', icon: 'research', title: '在新窗口打开' }]
  : BASE_CONSUMER_ITEMS

const props = defineProps({
  stacked: { type: Boolean, default: false },
  items: { type: Array, default: null },
  brandTo: { type: String, default: '/app/research/new' },
  brandLabel: { type: String, default: '知股 Zhigu' },
  navLabel: { type: String, default: '主导航' }
})
const emit = defineEmits(['navigated'])
const route = useRoute()
const router = useRouter()
const conversation = useResearchConversation()
const intelRetryVisible = ref(false)
let lastIntelOpenAt = 0
const navItems = computed(() => props.items || CONSUMER_ITEMS)

const selected = computed(() => {
  if (props.items) {
    const hit = props.items.find((item) => item.match && route.path.startsWith(item.match))
    return hit?.key || ''
  }
  if (route.path.startsWith('/app/history')) return ''
  if (route.path.startsWith('/app/profile')) return 'profile'
  if (route.path.startsWith('/app/strategies')) return 'strategies'
  if (route.path.startsWith('/app/research')) return 'research'
  return ''
})

function go(item) {
  if (item.key === 'intel') {
    const now = Date.now()
    if (now - lastIntelOpenAt < 500) return
    lastIntelOpenAt = now
    intelRetryVisible.value = true
    openIntelWindow()
    emit('navigated')
    return
  }
  if (item.to) {
    if (route.path !== item.to) router.push(item.to)
    emit('navigated')
    return
  }
  if (item.key === 'research') {
    const runId = conversation.currentRunId
    const target = runId ? `/app/research/${runId}` : '/app/research/new'
    if (route.path !== target) router.push(target)
    emit('navigated')
    return
  }
  const paths = { profile: '/app/profile', strategies: '/app/strategies' }
  router.push(paths[item.key])
  emit('navigated')
}
</script>
<style scoped>
.intel-retry-link {
  display: block;
  margin: 4px 12px 10px;
  color: var(--zg-action);
  font-size: 12px;
}
.side-wrap {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--zg-surface);
}
.stacked { width: 100%; }
.stacked .zhigu-side-nav { width: 100%; }
.stacked .zhigu-brand { align-items: flex-start; padding: 8px 12px; }
.stacked .zhigu-nav-item {
  width: auto;
  min-height: 44px;
  flex-direction: row;
  justify-content: flex-start;
  gap: 12px;
  margin: 0 8px 8px;
  padding: 10px 12px;
}
</style>
