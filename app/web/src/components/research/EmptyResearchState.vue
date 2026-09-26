<template>
  <div class="empty" :class="{ compact }">
    <img :src="emptyArtwork" width="160" height="120" alt="" class="artwork" />
    <h2 v-if="title">{{ title }}</h2>
    <p>{{ description }}</p>
    <slot />
  </div>
</template>
<script setup>
import emptyArtwork from '../../assets/brand/empty-research.svg'
defineProps({
  title: { type: String, default: '' },
  description: { type: String, default: '还没有内容。' },
  compact: { type: Boolean, default: false }
})
</script>
<style scoped>
.empty {
  display: grid;
  justify-items: center;
  text-align: center;
  gap: 12px;
  padding: 56px 24px;
  color: var(--zg-text-secondary);
}
.artwork { display: block; margin-bottom: 8px; }
.compact { padding: 24px 16px; }
.compact .artwork { width: 112px; height: 84px; }
h2 {
  margin: 8px 0 0;
  font-size: 22px;
  line-height: 32px;
  color: var(--zg-ink);
  font-weight: 600;
  font-family: var(--zg-font-editorial);
}
p { margin: 0; font-size: 14px; line-height: 22px; }
</style>
