<script setup>
defineProps({
  environment: { type: String, required: true },
  provenance: { type: String, required: true },
  reason: { type: String, default: '' },
})
</script>

<template>
  <aside
    v-if="environment !== 'production' || provenance !== 'real'"
    class="mode-banner"
    role="status"
  >
    {{ environment }} · {{ provenance }}<template v-if="reason"> · {{ reason }}</template>
  </aside>
</template>
