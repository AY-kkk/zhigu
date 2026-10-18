<template>
  <main id="main-content" ref="main" class="futures-shell" tabindex="-1">
    <a class="skip-link" href="#main-content" @click.prevent="focusMain">跳到主要内容</a>
    <SideNav class="futures-main-nav" />
    <header class="futures-header">
      <div>
        <router-link class="back-link" to="/app/research/new">返回投研观点</router-link>
        <h1>期货研究</h1>
      </div>
      <router-link class="admin-link" to="/admin/futures/sources">数据管理</router-link>
    </header>
    <FuturesNav />
    <router-view v-slot="{ Component }">
      <keep-alive :include="['OverviewPage']"><component :is="Component" /></keep-alive>
    </router-view>
  </main>
</template>

<script setup>
import { ref } from 'vue'
import FuturesNav from '../components/FuturesNav.vue'
import SideNav from '../../../components/workspace/SideNav.vue'

const main = ref(null)

function focusMain() {
  main.value?.focus({ preventScroll: true })
}
</script>

<style scoped>
.futures-shell { min-height: 100vh; padding: 24px; color: var(--zg-ink); background: var(--zg-surface); }
.skip-link { position: fixed; inset-inline-start: 16px; inset-block-start: -64px; z-index: 1000; padding: 10px 14px; background: var(--zg-ink, #272329); color: white; }
.skip-link:focus { inset-block-start: 12px; }
.futures-header { max-width: 1200px; margin: 0 auto; display: flex; justify-content: space-between; gap: 16px; align-items: start; }
.futures-header h1 { margin: 4px 0 0; font-size: clamp(28px, 4vw, 48px); }
.back-link, .admin-link { color: var(--zg-action, #9f2436); }
.futures-nav { max-width: 1200px; margin: auto; }
.futures-shell :deep(.futures-page) { max-width: 1200px; margin: auto; }
@media (max-width: 600px) { .futures-shell { padding: 16px; } .futures-header { align-items: center; } }
</style>
