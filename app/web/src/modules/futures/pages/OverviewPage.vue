<template>
  <section class="futures-page overview">
    <header><p class="eyebrow">CU 单品种研究</p><h2>研究首页</h2><p>只展示已登记来源、冻结版本与条件性结论，不生成无证据市场方向。</p></header>
    <p v-if="store.error" class="error" role="alert">{{ store.error.message || '期货研究服务暂不可用' }}</p>
    <section class="status-grid">
      <article><h3>运行状态</h3><strong>{{ statusText }}</strong><p>{{ store.capabilities?.reason || '等待能力检查' }}</p></article>
      <article><h3>关注品种</h3><RouterLink to="/app/futures/products/SHFE.CU">SHFE.CU 工作台</RouterLink></article>
      <article><h3>待验证假设</h3><strong>{{ store.hypotheses.filter(item => item.needs_review).length }}</strong></article>
      <article><h3>未读通知</h3><strong>{{ store.notifications.filter(item => !item.read_at).length }}</strong><RouterLink to="/app/futures/notifications">查看通知</RouterLink></article>
    </section>
    <section><h3>已收录变化</h3><ul><li v-for="item in changes" :key="item.id"><RouterLink :to="`/app/futures/hypotheses/${item.object_id}`">{{ item.type }}</RouterLink><time>{{ item.created_at }}</time></li><li v-if="!changes.length">当前没有带来源和关联假设的变化记录。</li></ul></section>
    <RouterLink class="primary" to="/app/futures/research/new">新建期货研究</RouterLink>
  </section>
</template>

<script setup>
import { computed, onMounted } from 'vue'
import { useFuturesStore } from '../store.js'

const store = useFuturesStore()
const statusText = computed(() => store.capabilities?.ready ? `${store.capabilities.mode} 已就绪` : '未开放')
const changes = computed(() => store.notifications.filter(item => ['check_changed', 'revision', 'retraction'].includes(item.type)))
onMounted(async () => {
  await Promise.allSettled([store.loadCapabilities(), store.loadHypotheses(), store.loadNotifications()])
})
</script>

<style scoped>
.overview header { max-width: 760px; }.overview h2 { font-size: clamp(32px, 5vw, 64px); margin: 0; }.eyebrow { color: var(--zg-action); font-weight: 700; }
.status-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin: 28px 0; }.status-grid article { padding: 18px; border: 1px solid #ddd; }.status-grid h3 { margin-top: 0; }
.primary { display: inline-block; padding: 12px 18px; background: var(--zg-action); color: white; }.error { color: #b42318; } time { display: block; color: var(--zg-muted); }
@media (max-width: 800px) { .status-grid { grid-template-columns: 1fr 1fr; } } @media (max-width: 480px) { .status-grid { grid-template-columns: 1fr; } }
</style>
