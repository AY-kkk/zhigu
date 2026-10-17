<script setup>
defineProps({
  title: { type: String, required: true },
  description: { type: String, required: true },
  actionLabel: { type: String, default: '' },
})

defineEmits(['action'])
</script>

<template>
  <section class="empty-state">
    <h2>{{ title }}</h2>
    <p>{{ description }}</p>
    <button v-if="actionLabel" type="button" @click="$emit('action')">
      {{ actionLabel }}
    </button>
  </section>
</template>
