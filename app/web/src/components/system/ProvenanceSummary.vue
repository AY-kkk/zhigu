<script setup>
defineProps({
  source: { type: String, required: true },
  version: { type: String, default: '' },
  license: { type: String, default: '' },
})
</script>

<template>
  <dl class="provenance-summary">
    <dt>来源</dt>
    <dd>{{ source }}</dd>
    <template v-if="version">
      <dt>版本</dt>
      <dd>{{ version }}</dd>
    </template>
    <template v-if="license">
      <dt>权限</dt>
      <dd>{{ license }}</dd>
    </template>
  </dl>
</template>
