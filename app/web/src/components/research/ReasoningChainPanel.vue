<template>
  <div class="reasoning-gaps">
    <article v-for="item in items" :key="item.claim_id" class="gap">
      <p class="chain"><strong>{{ item.from }}</strong> → <strong>{{ item.to }}</strong></p>
      <p>{{ item.missing }}</p>
    </article>
    <p v-if="!items.length" class="empty">未识别到额外推理链缺口。</p>
  </div>
</template>
<script setup>
defineProps({ items: { type: Array, default: () => [] } })
</script>
<style scoped>
.reasoning-gaps { display: grid; gap: 12px; }
.gap { padding: 12px 14px; border-left: 3px solid var(--zg-brand); background: var(--zg-surface); }
.chain, .gap p { margin: 0; line-height: 26px; }
.empty { color: var(--zg-text-secondary); margin: 0; }
</style>
