<template>
  <article class="card">
    <header>
      <h3>
        <RouterLink :to="`/app/strategies/market/${item.id}`" class="title">{{ item.name }}</RouterLink>
      </h3>
      <span class="ver">v{{ item.version_no }}</span>
    </header>
    <p class="summary">{{ item.summary }}</p>
    <p class="chips">
      <span class="chip">{{ item.category }}</span>
      <span v-for="m in marketList" :key="m" class="chip">{{ m === 'A' ? 'A股' : '港股' }}</span>
      <span class="chip">{{ item.signal_period }}</span>
      <span v-for="t in tagList" :key="t" class="chip ghost">{{ t }}</span>
    </p>
    <p class="status">
      <span class="badge" :class="item.validation_status">规则校验：{{ validationText }}</span>
      <span class="badge" :class="item.backtest_status">回测证据：{{ backtestText }}</span>
    </p>
    <p class="meta">
      <span>更新于 {{ dateText(item.updated_at) }}</span>
      <span v-if="!item.copyable" class="reason">{{ item.copy_disabled_reason }}</span>
      <span v-else class="ok">可复制为我的策略</span>
    </p>
  </article>
</template>

<script setup>
import { computed } from 'vue'
import { RouterLink } from 'vue-router'

const props = defineProps({
  item: { type: Object, required: true }
})

const tagList = computed(() => (Array.isArray(props.item.tags) ? props.item.tags.slice(0, 3) : []))
const marketList = computed(() => (Array.isArray(props.item.markets) ? props.item.markets : []))
const validationText = computed(
  () => ({ passed: '已通过', failed: '未通过', pending: '未校验' }[props.item.validation_status] || '未校验')
)
// 只展示有依据状态：无 approved 证据一律显示「未回测」，不填收益数字。
const backtestText = computed(() => (props.item.backtest_status === 'has_evidence' ? '有历史模拟记录' : '未回测'))

function dateText(v) {
  return typeof v === 'string' ? v.slice(0, 10) : ''
}
</script>

<style scoped>
.card {
  border: 1px solid var(--zg-line, #e5e7eb);
  border-radius: 12px;
  padding: 14px 16px;
  background: var(--zg-panel, #fff);
}
header {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
h3 {
  margin: 0;
  font-size: 16px;
}
.title {
  color: inherit;
  text-decoration: none;
}
.title:hover {
  text-decoration: underline;
}
.ver {
  font-size: 12px;
  color: var(--zg-muted, #6b7280);
}
.summary {
  margin: 6px 0;
  font-size: 13px;
  color: var(--zg-text, #111827);
}
.chips {
  margin: 6px 0;
}
.chip {
  display: inline-block;
  margin-right: 6px;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
  background: #f1f5f9;
}
.chip.ghost {
  background: transparent;
  border: 1px solid var(--zg-line, #e5e7eb);
  color: var(--zg-muted, #6b7280);
}
.status {
  margin: 6px 0;
}
.badge {
  margin-right: 8px;
  padding: 2px 8px;
  border-radius: 6px;
  font-size: 12px;
  background: #fef9c3;
}
.badge.passed,
.badge.has_evidence {
  background: #dcfce7;
}
.badge.failed {
  background: #fee2e2;
}
.meta {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--zg-muted, #6b7280);
  display: flex;
  gap: 12px;
}
.reason {
  color: #b45309;
}
.ok {
  color: #15803d;
}
</style>
