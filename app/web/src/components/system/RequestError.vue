<script setup>
defineProps({
  error: { type: Object, required: true },
})

defineEmits(['retry'])
</script>

<template>
  <section class="system-error" role="alert">
    <strong>{{ error.code }}</strong>
    <p>{{ error.message }}</p>
    <small v-if="error.traceId">trace_id：{{ error.traceId }}</small>
    <button v-if="error.retryable" type="button" @click="$emit('retry')">重试</button>
  </section>
</template>
