<script setup>
import { RouterLink } from 'vue-router'

defineProps({
  error: {
    type: Object,
    default: () => ({
      code: 'ROUTE_NOT_FOUND',
      message: '页面不存在或已被移动',
      traceId: '',
    }),
  },
  homeLabel: { type: String, default: '返回工作台' },
})

defineEmits(['retry'])
</script>

<template>
  <main class="route-error" role="main">
    <p class="route-error-code">{{ error.code }}</p>
    <h1>暂时无法打开这个页面</h1>
    <p>{{ error.message }}</p>
    <small v-if="error.traceId">trace_id：{{ error.traceId }}</small>
    <div>
      <RouterLink to="/app">{{ homeLabel }}</RouterLink>
      <button v-if="error.retryable" type="button" @click="$emit('retry')">重试</button>
    </div>
  </main>
</template>

<style scoped>
.route-error { max-width: 680px; margin: 12vh auto; padding: 24px; }
.route-error-code { color: var(--zg-error-fg, #a62f35); font-weight: 700; }
.route-error div { display: flex; gap: 12px; margin-top: 24px; }
.route-error a, .route-error button { min-height: 42px; padding: 10px 14px; border: 1px solid currentColor; border-radius: 8px; }
</style>
