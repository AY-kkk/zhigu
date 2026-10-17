<script setup>
defineProps({
  asOf: { type: String, default: '' },
  updatedAt: { type: String, default: '' },
  stale: { type: Boolean, default: false },
  reason: { type: String, default: '' },
})
</script>

<template>
  <p class="data-freshness" role="status">
    <span>数据截止：{{ asOf || '未提供' }}</span>
    <span>更新：{{ updatedAt || '未提供' }}</span>
    <span v-if="stale" class="system-status" data-tone="warning">
      陈旧{{ reason ? `：${reason}` : '' }}
    </span>
  </p>
</template>
