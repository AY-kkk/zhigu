<script setup>
defineProps({
  open: { type: Boolean, required: true },
  title: { type: String, required: true },
  impact: { type: String, required: true },
  confirmLabel: { type: String, default: '确认' },
  busy: { type: Boolean, default: false },
})

defineEmits(['confirm', 'cancel'])
</script>

<template>
  <div
    v-if="open"
    class="confirm-action"
    role="dialog"
    aria-modal="true"
    :aria-label="title"
  >
    <h2>{{ title }}</h2>
    <p>{{ impact }}</p>
    <button type="button" :disabled="busy" @click="$emit('cancel')">取消</button>
    <button type="button" :disabled="busy" @click="$emit('confirm')">
      {{ confirmLabel }}
    </button>
  </div>
</template>
