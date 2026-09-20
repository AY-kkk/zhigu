<template>
  <a v-if="href" :href="href" rel="noreferrer noopener" target="_blank"><slot>{{ href }}</slot></a>
  <span v-else>{{ fallback }}</span>
</template>
<script setup>
import { computed } from 'vue'
import { sanitizeHref } from '../../utils/sanitize.js'
const props = defineProps({
  href: { type: String, default: '' }
})
const href = computed(() => sanitizeHref(props.href))
const fallback = computed(() => (props.href ? '链接已拦截' : '暂未提供'))
</script>
<style scoped>
a, span { overflow-wrap: anywhere; word-break: break-all; }
</style>
