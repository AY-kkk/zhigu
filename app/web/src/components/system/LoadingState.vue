<script setup>
defineProps({
  label: { type: String, default: '正在加载' },
  rows: { type: Number, default: 3 },
})
</script>

<template>
  <div class="loading-state" role="status" aria-live="polite">
    <span>{{ label }}</span>
    <div
      v-for="row in rows"
      :key="row"
      class="loading-row"
      :style="{ width: `${92 - row * 8}%` }"
    />
  </div>
</template>
