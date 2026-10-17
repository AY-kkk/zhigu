<script setup>
import { ref } from 'vue'

defineProps({
  mobile: { type: Boolean, default: false },
  capabilities: { type: Object, default: null },
  environment: { type: String, default: '' },
  user: { type: Object, default: null },
})

defineEmits(['toggle-navigation', 'navigate', 'logout'])

const main = ref(null)

function focusPageTitle() {
  const root = main.value
  if (!root) return
  const title = root.querySelector('#page-title') || root.querySelector('h1')
  if (!title) return
  if (!title.hasAttribute('tabindex')) title.setAttribute('tabindex', '-1')
  title.focus({ preventScroll: true })
}

defineExpose({ focusPageTitle })
</script>

<template>
  <div class="app-shell" :class="{ 'is-mobile': mobile }">
    <a class="skip-link" href="#main-content">跳到主要内容</a>
    <div class="app-shell-inner">
      <aside v-if="!mobile" class="app-shell-side">
        <slot name="side" />
      </aside>
      <div class="app-shell-main">
        <header class="app-shell-header">
          <slot name="header" />
        </header>
        <main id="main-content" ref="main" tabindex="-1" class="app-shell-content">
          <slot />
        </main>
      </div>
    </div>
    <slot name="overlays" />
  </div>
</template>

<style scoped>
.app-shell {
  min-height: 100dvh;
  background: var(--zg-paper, #f7f5f0);
  color: var(--zg-ink, #272329);
}
.app-shell-inner {
  display: grid;
  grid-template-columns: 240px minmax(0, 1fr);
  min-height: 100dvh;
}
.app-shell-side {
  position: sticky;
  top: 0;
  height: 100dvh;
  overflow-y: auto;
  border-inline-end: 1px solid var(--zg-line, #dedbd4);
  background: var(--zg-surface, #fffefb);
}
.app-shell-main {
  min-width: 0;
}
.app-shell-header {
  position: sticky;
  top: 0;
  z-index: 20;
  min-height: 64px;
  padding: 10px clamp(16px, 3vw, 32px);
  border-bottom: 1px solid var(--zg-line, #dedbd4);
  background: color-mix(in srgb, var(--zg-surface, #fffefb) 94%, transparent);
  backdrop-filter: blur(12px);
}
.app-shell-content {
  width: min(100%, 1440px);
  margin-inline: auto;
  padding: clamp(20px, 3vw, 40px);
  outline: none;
}
.skip-link {
  position: fixed;
  inset-inline-start: 16px;
  inset-block-start: -64px;
  z-index: 1000;
  padding: 10px 14px;
  background: var(--zg-ink, #272329);
  color: white;
}
.skip-link:focus { inset-block-start: 12px; }
@media (max-width: 1279px) {
  .app-shell-inner { grid-template-columns: 208px minmax(0, 1fr); }
}
@media (max-width: 767px) {
  .app-shell-inner { display: block; }
  .app-shell-content { padding: 18px 14px 88px; }
}
</style>
