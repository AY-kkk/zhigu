<template>
  <nav class="subnav" aria-label="交易策略导航">
    <RouterLink class="tab" :class="{ on: section === 'market' }" to="/app/strategies/market">策略市场</RouterLink>
    <RouterLink class="tab" :class="{ on: section === 'workspace' }" to="/app/strategies">策略工作台</RouterLink>
  </nav>
</template>

<script setup>
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'

const route = useRoute()
const section = computed(() => route.meta?.strategySection || 'workspace')
</script>

<style scoped>
.subnav {
  display: flex;
  gap: 8px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--zg-line, #e5e7eb);
  background: var(--zg-panel, #fff);
}
.tab {
  padding: 6px 14px;
  border-radius: 999px;
  font-size: 14px;
  color: var(--zg-muted, #6b7280);
  text-decoration: none;
}
.tab.on {
  color: var(--zg-brand, #1d4ed8);
  background: color-mix(in srgb, var(--zg-brand, #1d4ed8) 10%, transparent);
  font-weight: 600;
}
</style>
