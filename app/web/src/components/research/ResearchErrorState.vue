<template>
  <div class="err" role="alert">
    <ZhiguIcon name="error" :size="20" />
    <div class="body">
      <p class="title">{{ title }}</p>
      <p class="msg">{{ message }}</p>
      <div class="actions">
        <button v-if="retryLabel" type="button" class="zg-btn zg-btn-secondary" @click="$emit('retry')">{{ retryLabel }}</button>
        <slot />
      </div>
    </div>
  </div>
</template>
<script setup>
import ZhiguIcon from '../brand/ZhiguIcon.vue'
defineProps({
  title: { type: String, default: '请求失败' },
  message: { type: String, default: '' },
  retryLabel: { type: String, default: '重试' }
})
defineEmits(['retry'])
</script>
<style scoped>
.err {
  display: flex;
  gap: 12px;
  padding: 16px;
  background: var(--zg-error-bg);
  color: var(--zg-error-fg);
  border: 1px solid var(--zg-error-fg);
  border-radius: var(--zg-radius-card);
}
.body { min-width: 0; flex: 1; }
.title { margin: 0; font-weight: 600; font-size: 14px; line-height: 22px; }
.msg { margin: 4px 0 0; font-size: 14px; line-height: 22px; color: var(--zg-ink); }
.actions { display: flex; gap: 8px; margin-top: 12px; flex-wrap: wrap; }
</style>
