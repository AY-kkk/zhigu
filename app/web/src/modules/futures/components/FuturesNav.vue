<template>
  <nav class="futures-nav" aria-label="期货研究导航">
    <RouterLink v-for="item in items" :key="item.to" :to="item.to" :class="{ active: isActive(item) }">
      {{ item.text }}
    </RouterLink>
  </nav>
</template>

<script setup>
import { useRoute } from 'vue-router'

const route = useRoute()
const items = [
  { text: '研究首页', to: '/app/futures', exact: true },
  { text: '新建研究', to: '/app/futures/research/new' },
  { text: '通知', to: '/app/futures/notifications' }
]

function isActive(item) {
  return item.exact ? route.path === item.to : route.path.startsWith(item.to)
}
</script>

<style scoped>
.futures-nav { display: flex; gap: 8px; overflow-x: auto; padding: 8px 0 14px; }
.futures-nav a { flex: 0 0 auto; padding: 8px 12px; color: var(--zg-muted, #667085); border-bottom: 2px solid transparent; text-decoration: none; }
.futures-nav a.active { color: var(--zg-action, #9f2436); border-color: currentColor; font-weight: 700; }
</style>
