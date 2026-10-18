<script setup>
defineProps({
  title: { type: String, required: true },
  summary: { type: String, default: '' },
  status: { type: String, default: '' },
  primaryAction: { type: Object, default: null },
  secondaryActions: { type: Array, default: () => [] },
})

defineEmits(['primary', 'secondary'])
</script>

<template>
  <header class="page-header">
    <div class="page-header-copy">
      <h1 id="page-title" tabindex="-1">{{ title }}</h1>
      <p v-if="summary">{{ summary }}</p>
      <slot name="status">
        <span v-if="status" class="system-status" data-tone="neutral">{{ status }}</span>
      </slot>
    </div>
    <div class="page-header-actions">
      <button
        v-for="(action, index) in secondaryActions"
        :key="action.label"
        type="button"
        class="secondary"
        @click="$emit('secondary', action, index)"
      >
        {{ action.label }}
      </button>
      <button
        v-if="primaryAction"
        type="button"
        class="primary"
        :disabled="primaryAction.disabled"
        @click="$emit('primary', primaryAction)"
      >
        {{ primaryAction.label }}
      </button>
    </div>
  </header>
</template>

<style scoped>
.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 28px;
}
.page-header h1 { margin: 0; font-size: clamp(26px, 3vw, 38px); line-height: 1.18; }
.page-header p { max-width: 760px; margin: 8px 0 0; color: var(--zg-muted, #68616b); }
.page-header-actions { display: flex; gap: 8px; flex-wrap: wrap; }
button { min-height: 40px; padding: 8px 14px; border-radius: 8px; border: 1px solid var(--zg-line, #dedbd4); }
button.primary { border-color: var(--zg-action, #b4232c); background: var(--zg-action, #b4232c); color: white; }
button:disabled { cursor: not-allowed; opacity: .55; }
@media (max-width: 767px) {
  .page-header { display: block; }
  .page-header-actions { margin-top: 16px; }
}
</style>
