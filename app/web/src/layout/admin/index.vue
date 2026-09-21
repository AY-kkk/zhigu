<template>
  <ConfigProvider :locale="zhCN" :getPopupContainer="getConsumerPopupContainer">
    <div class="zhigu-consumer zhigu-admin zhigu-app-shell">
      <SkipLink />
      <div class="zhigu-shell-inner">
        <Layout>
          <LayoutSider v-if="!isMobile" class="zhigu-sider" :width="navWidth">
            <SideNav
              :items="navItems"
              brand-to="/admin/ai-settings"
              brand-label="知股管理"
              nav-label="后台导航"
            />
          </LayoutSider>
          <Layout class="zhigu-main">
            <LayoutHeader class="zhigu-header">
              <div class="admin-header">
                <button
                  v-if="isMobile"
                  type="button"
                  class="zg-btn zg-btn-ghost menu-btn"
                  aria-label="打开导航"
                  @click="navOpen = true"
                >
                  <ZhiguIcon name="menu" :size="20" />
                </button>
                <p class="zhigu-header-title title">{{ title }}</p>
                <div class="actions">
                  <Dropdown trigger="click" position="bottomRight" :getPopupContainer="getConsumerPopupContainer" :menu="menu">
                    <button type="button" class="zg-btn zg-btn-ghost" aria-label="更多操作">
                      <ZhiguIcon name="more" :size="20" />
                    </button>
                  </Dropdown>
                </div>
              </div>
            </LayoutHeader>
            <LayoutContent class="zhigu-content is-scrollable">
              <div id="zg-main" class="zg-main-inner" tabindex="-1">
                <FixtureBanner />
                <router-view />
              </div>
            </LayoutContent>
          </Layout>
        </Layout>
      </div>
      <SideSheet
        v-if="isMobile"
        className="zhigu-consumer zhigu-mobile-nav"
        title="知股管理"
        :visible="navOpen"
        placement="left"
        :width="260"
        :closeOnEsc="true"
        :closeIcon="navCloseIcon"
        :zIndex="210"
        :motion="false"
        :getPopupContainer="getConsumerPopupContainer"
        :onCancel="() => navOpen = false"
      >
        <SideNav
          stacked
          :items="navItems"
          brand-to="/admin/ai-settings"
          brand-label="知股管理"
          nav-label="后台导航"
          @navigated="navOpen = false"
        />
      </SideSheet>
      <Teleport to="body">
        <div id="zhigu-overlay-root" class="zhigu-consumer zhigu-overlay-root"></div>
      </Teleport>
    </div>
  </ConfigProvider>
</template>
<script setup>
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ConfigProvider,
  Dropdown,
  Layout,
  LayoutContent,
  LayoutHeader,
  LayoutSider,
  SideSheet
} from '@kousum/semi-ui-vue'
import { IconClose } from '@kousum/semi-icons-vue'
import zhCN from '@kousum/semi-ui-vue/dist/locale/source/zh_CN.js'
import SkipLink from '../../components/brand/SkipLink.vue'
import SideNav from '../../components/workspace/SideNav.vue'
import ZhiguIcon from '../../components/brand/ZhiguIcon.vue'
import FixtureBanner from '../../components/research/FixtureBanner.vue'
import { useSession } from '../../stores/session.js'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { getConsumerPopupContainer } from '../../utils/popup.js'
import '@kousum/semi-ui-vue/dist/_base/base.css'
import '../../styles/consumer-semi.css'
import '../../styles/admin-element.css'

const route = useRoute()
const router = useRouter()
const session = useSession()
const conversation = useResearchConversation()
const navOpen = ref(false)
const navCloseIcon = h(IconClose, { 'aria-label': '关闭导航' })
const viewport = ref(typeof window === 'undefined' ? 1440 : window.innerWidth)
const isMobile = computed(() => viewport.value < 768)
const navWidth = computed(() => (viewport.value >= 768 && viewport.value < 1024 ? 72 : 88))
const title = computed(() => (route.path.startsWith('/admin/research-runs') ? '运行记录' : '模型与数据源'))
const navItems = computed(() => {
  const runId = conversation.currentRunId
  return [
    { key: 'settings', text: '模型配置', icon: 'settings', to: '/admin/ai-settings', match: '/admin/ai-settings' },
    { key: 'runs', text: '运行记录', icon: 'queue', to: '/admin/research-runs', match: '/admin/research-runs' },
    { key: 'client', text: '返回客户', icon: 'research', to: runId ? `/app/research/${runId}` : '/app/research/new' }
  ]
})
const menu = computed(() => [
  { node: 'item', name: '退出登录', onClick: onLogout }
])

function updateViewport() {
  viewport.value = window.innerWidth
}
onMounted(() => {
  updateViewport()
  window.addEventListener('resize', updateViewport)
})
onBeforeUnmount(() => window.removeEventListener('resize', updateViewport))

function onLogout() {
  conversation.resetAll()
  session.logout()
  router.push('/login')
}
</script>
<style scoped>
.admin-header {
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
@media (max-width: 767px) {
  .title { font-size: 14px; }
}
</style>
