<template>
  <svg
    class="zg-icon"
    :width="px"
    :height="px"
    :viewBox="isQuote ? '0 0 96 84' : '0 0 24 24'"
    fill="none"
    aria-hidden="true"
    focusable="false"
  >
    <use v-if="isQuote" :href="`${symbol}#zhigu-mark`" />
    <g v-else-if="name === 'research'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <rect x="5" y="3" width="14" height="18" rx="2" />
      <path d="M8 9h3M8 13h8M8 17h6M14 6h2" />
    </g>
    <g v-else-if="name === 'account'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
      <circle cx="12" cy="8" r="3.2" />
      <path d="M5.5 19c1.4-3 3.8-4.5 6.5-4.5S16.1 16 17.5 19" />
    </g>
    <g v-else-if="name === 'strategies'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <path d="M4 16l5-5 4 3 7-8" />
      <path d="M15 6h5v5" />
    </g>
    <g v-else-if="name === 'history'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <circle cx="12" cy="12" r="8" />
      <path d="M12 8v5l3 2" />
    </g>
    <g v-else-if="name === 'plus'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
      <path d="M12 5v14M5 12h14" />
    </g>
    <g v-else-if="name === 'more'" fill="currentColor">
      <circle cx="6" cy="12" r="1.6" />
      <circle cx="12" cy="12" r="1.6" />
      <circle cx="18" cy="12" r="1.6" />
    </g>
    <g v-else-if="name === 'menu'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
      <path d="M4 7h16M4 12h16M4 17h16" />
    </g>
    <g v-else-if="name === 'settings'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
      <path d="M4 8h16M4 16h16" />
      <circle cx="9" cy="8" r="2.2" fill="currentColor" stroke="none" />
      <circle cx="15" cy="16" r="2.2" fill="currentColor" stroke="none" />
    </g>
    <g v-else-if="name === 'arrow'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <path d="M5 12h14M13 6l6 6-6 6" />
    </g>
    <g v-else-if="name === 'close'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
      <path d="M6 6l12 12M18 6L6 18" />
    </g>
    <g v-else-if="name === 'copy'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <rect x="8" y="8" width="11" height="11" rx="1.5" />
      <path d="M5 15V5.5A1.5 1.5 0 0 1 6.5 4H15" />
    </g>
    <g v-else-if="name === 'refresh'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <path d="M20 12a8 8 0 1 1-2.2-5.6" />
      <path d="M20 5v5h-5" />
    </g>
    <g v-else-if="name === 'search'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
      <circle cx="11" cy="11" r="6.5" />
      <path d="M16 16l4 4" />
    </g>
    <g v-else-if="name === 'queue'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
      <path d="M4 7h16M4 12h12M4 17h8" />
    </g>
    <g v-else-if="name === 'verify'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <circle cx="12" cy="5" r="2.4" />
      <circle cx="5" cy="18" r="2.4" />
      <circle cx="19" cy="18" r="2.4" />
      <path d="M10.8 7.2L6.2 15.8M13.2 7.2l4.6 8.6M7.5 18h9" />
    </g>
    <g v-else-if="name === 'report'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <path d="M7 4h7l4 4v12a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z" />
      <path d="M14 4v4h4M9 13h6M9 17h4" />
    </g>
    <g v-else-if="name === 'check'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <path d="M5 12.5l4.2 4.2L19 7.5" />
    </g>
    <g v-else-if="name === 'warning'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 4l9 16H3L12 4z" />
      <path d="M12 10v4M12 16.5v.5" />
    </g>
    <g v-else-if="name === 'info'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
      <circle cx="12" cy="12" r="8" />
      <path d="M12 11v5M12 8v.5" />
    </g>
    <g v-else-if="name === 'error'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
      <circle cx="12" cy="12" r="8" />
      <path d="M12 8v5M12 16.5v.5" />
    </g>
    <g v-else-if="name === 'offline'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <path d="M5 12a9.5 9.5 0 0 1 14 0M8 15a5.5 5.5 0 0 1 8 0" />
      <circle cx="12" cy="18.5" r="1" fill="currentColor" />
      <path d="M4 5l16 14" />
    </g>
    <g v-else-if="name === 'empty'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <path d="M6 7h12a1 1 0 0 1 1 1v11l-3-2-3 2-3-2-3 2V8a1 1 0 0 1 1-1z" />
      <path d="M9 4h6" />
    </g>
    <g v-else-if="name === 'mixed'">
      <circle cx="10" cy="12" r="6" stroke="currentColor" stroke-width="1.8" fill="none" />
      <circle cx="14" cy="12" r="6" stroke="currentColor" stroke-width="1.8" fill="none" />
    </g>
    <g v-else-if="name === 'insufficient'">
      <circle cx="12" cy="12" r="8" stroke="currentColor" stroke-width="1.8" stroke-dasharray="3 3" fill="none" />
    </g>
    <g v-else-if="name === 'external'" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <path d="M10 6H6a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-4" />
      <path d="M14 4h6v6M12 12l8-8" />
    </g>
    <g v-else stroke="currentColor" stroke-width="1.8">
      <circle cx="12" cy="12" r="8" />
    </g>
  </svg>
</template>
<script setup>
import { computed } from 'vue'
import symbol from '../../assets/brand/zhigu-symbol.svg?no-inline'
const props = defineProps({
  name: { type: String, required: true },
  size: { type: [Number, String], default: 20 }
})
const px = computed(() => Number(props.size) || 20)
const isQuote = computed(() => ['quote', 'support', 'challenge'].includes(props.name))
</script>
<style scoped>
.zg-icon { display: block; flex: none; }
</style>
