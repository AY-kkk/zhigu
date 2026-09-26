<template>
  <ConfigProvider :locale="zhCN" :getPopupContainer="getPopup">
    <div class="intel-shell">
      <header class="intel-header">
        <div class="intel-brand">
          <div class="intel-brand-mark">知</div>
          <div>
            <p class="intel-kicker">知股 / EVENT INTELLIGENCE</p>
            <h1>事件情报与证据时间线</h1>
          </div>
        </div>
        <div class="intel-header-right">
          <div class="intel-header-meta">
            <span class="intel-mode" :class="mode === 'demo' ? 'is-demo' : 'is-live'">{{ mode === 'demo' ? '演示数据' : '真实数据' }}</span>
            <span class="intel-disclaimer">仅整理信息，不构成投资建议</span>
          </div>
          <div class="intel-window-actions">
            <button type="button" class="zg-btn zg-btn-ghost" @click="goEvents">返回事件流</button>
            <button type="button" class="zg-btn zg-btn-ghost" @click="closeWindow">关闭窗口</button>
            <button type="button" class="zg-btn zg-btn-ghost" @click="logout">{{ mode === 'demo' ? '结束演示' : '退出登录' }}</button>
          </div>
          <p v-if="closeHint" class="intel-close-hint">{{ closeHint }}</p>
        </div>
      </header>
      <nav class="intel-tabs" aria-label="事件情报导航">
        <button
          v-for="tab in visibleTabs"
          :key="tab.key"
          type="button"
          class="intel-tab"
          :class="{ 'is-active': workspace.activeTab === tab.key || (tab.key === 'events' && workspace.activeTab === 'detail') }"
          @click="goTab(tab)"
        >
          {{ tab.label }}<span v-if="tab.key === 'notifications' && workspace.unreadCount" class="intel-badge">{{ workspace.unreadCount }}</span>
        </button>
      </nav>
      <main class="intel-main">
        <router-view />
      </main>
      <footer class="intel-footer">数据覆盖、来源权利与处理状态均以当前空间为准；演示回放不会写入真实数据。</footer>
    </div>
  </ConfigProvider>
</template>
<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ConfigProvider } from '@kousum/semi-ui-vue'
import zhCN from '@kousum/semi-ui-vue/dist/locale/source/zh_CN.js'
import { useIntelWorkspace } from '../../stores/intelWorkspace.js'
import { safeInternalRedirect } from '../../router/finance.js'

const route = useRoute()
const router = useRouter()
const workspace = useIntelWorkspace()
const closeHint = ref('')
const mode = computed(() => route.meta?.mode === 'demo' ? 'demo' : 'live')
const visibleTabs = computed(() => {
  const tabs = [
    { key: 'events', label: '事件流', live: '/app/intel', demo: '/app/intel/demo' },
    { key: 'watchlist', label: '关注设置', live: '/app/intel/watchlist', demo: '/app/intel/demo/watchlist' },
    { key: 'notifications', label: '通知中心', live: '/app/intel/notifications', demo: '/app/intel/demo/notifications' }
  ]
  if (mode.value === 'demo') tabs.push({ key: 'replay', label: '回放', demo: '/app/intel/demo' })
  if (mode.value === 'live' && localStorage.getItem('zhigu_role') === 'admin') {
    tabs.push({ key: 'reviews', label: '待处理', live: '/app/intel/admin/reviews' })
    tabs.push({ key: 'sources', label: '数据接入', live: '/app/intel/admin/sources' })
  }
  return tabs
})

function goTab(tab) {
  if (tab.key === 'replay') {
    workspace.activeTab = 'replay'
    return
  }
  const target = mode.value === 'demo' ? tab.demo : tab.live
  if (target && route.path !== target) router.push(target)
}
function goEvents() {
  workspace.activeTab = 'events'
  router.push(mode.value === 'demo' ? '/app/intel/demo' : '/app/intel')
}
function closeWindow() {
  window.close()
  setTimeout(() => { closeHint.value = '浏览器可能禁止自动关闭；可直接关闭此标签页。' }, 150)
}
function logout() {
  if (mode.value === 'demo') {
    workspace.resetForAccount('demo')
    closeWindow()
    return
  }
  localStorage.removeItem('zhigu_token')
  localStorage.removeItem('zhigu_role')
  localStorage.removeItem('zhigu_user')
  workspace.resetForAccount('live')
  router.push('/login')
}
function onStorage(event) {
  if (!['zhigu_token', 'zhigu_user', 'zhigu_role'].includes(event.key)) return
  workspace.resetForAccount(mode.value)
  workspace.bootstrap(mode.value)
}
function onAuthExpired(event) {
  workspace.resetForAccount('live')
  router.push({ path: '/login', query: { redirect: safeInternalRedirect(event.detail) } })
}
let previousTitle = ''
let pollTimer = 0

onMounted(() => {
  previousTitle = document.title
  document.title = '知股 · 事件情报'
  workspace.bootstrap(mode.value)
  window.addEventListener('storage', onStorage)
  window.addEventListener('zhigu:intel-auth-expired', onAuthExpired)
  pollTimer = window.setInterval(() => {
    if (document.visibilityState === 'visible' && workspace.session) workspace.refreshVisible()
  }, 30000)
})
watch(() => route.meta?.intelTab, (value) => {
  if (value) workspace.activeTab = value
}, { immediate: true })
watch(() => route.meta?.mode, (value, previous) => {
  if (value && value !== previous) workspace.bootstrap(value)
})
onBeforeUnmount(() => {
  document.title = previousTitle
  window.clearInterval(pollTimer)
  window.removeEventListener('storage', onStorage)
  window.removeEventListener('zhigu:intel-auth-expired', onAuthExpired)
  workspace.eventsAbort?.abort()
  workspace.notificationsAbort?.abort()
})
function getPopup() { return document.body }
</script>
<style scoped>
.intel-shell { min-height: 100vh; background: #f7f6f3; color: var(--zg-ink); }
.intel-header { display: flex; justify-content: space-between; gap: 24px; align-items: center; padding: 28px clamp(18px, 5vw, 72px) 22px; background: #fffdfb; border-bottom: 1px solid #e9e4df; }
.intel-brand { display: flex; gap: 14px; align-items: center; }
.intel-brand-mark { display: grid; place-items: center; width: 44px; height: 44px; border-radius: 14px; background: #c94236; color: white; font-size: 24px; font-family: Georgia, serif; }
.intel-kicker { margin: 0 0 3px; color: #c94236; font-size: 10px; font-weight: 800; letter-spacing: .16em; }
.intel-header h1 { margin: 0; font-family: Georgia, 'Songti SC', serif; font-size: clamp(22px, 3vw, 32px); font-weight: 600; }
.intel-header-right { display: grid; justify-items: end; gap: 8px; }
.intel-header-meta, .intel-window-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px 10px; align-items: center; color: var(--zg-muted); font-size: 12px; }
.intel-mode { padding: 5px 9px; border-radius: 999px; font-weight: 700; }
.intel-mode.is-demo { background: #fff0ee; color: #b42318; }
.intel-mode.is-live { background: #e8f7ee; color: #18794e; }
.intel-close-hint { margin: 0; color: var(--zg-muted); font-size: 11px; }
.intel-tabs { display: flex; gap: 8px; padding: 14px clamp(18px, 5vw, 72px) 0; background: #fffdfb; overflow-x: auto; }
.intel-tab { position: relative; padding: 10px 14px; border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--zg-muted); cursor: pointer; white-space: nowrap; }
.intel-tab.is-active { border-color: #c94236; color: #c94236; font-weight: 700; }
.intel-badge { display: inline-grid; place-items: center; min-width: 18px; height: 18px; margin-left: 4px; padding: 0 5px; border-radius: 999px; background: #c94236; color: white; font-size: 10px; }
.intel-main { width: min(1280px, calc(100% - 36px)); margin: 0 auto; padding: 30px 0 60px; }
.intel-footer { padding: 18px; border-top: 1px solid #e9e4df; color: var(--zg-muted); text-align: center; font-size: 12px; }
@media (max-width: 767px) {
  .intel-header { align-items: flex-start; flex-direction: column; padding: 20px 16px 16px; }
  .intel-header-right { width: 100%; justify-items: start; }
  .intel-header-meta, .intel-window-actions { justify-content: flex-start; }
  .intel-tabs { padding-inline: 16px; }
  .intel-main { width: min(100% - 24px, 680px); padding-top: 20px; }
}
</style>
