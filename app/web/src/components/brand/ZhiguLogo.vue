<template>
  <span class="zg-logo" :class="[`is-${variant}`, `is-${tone}`]">
    <svg class="zg-logo-mark" viewBox="0 0 96 84" aria-hidden="true" focusable="false">
      <use :href="`${symbol}#zhigu-mark`" />
    </svg>
    <span v-if="variant !== 'symbol'" class="zg-logo-word">
      <span class="zg-logo-cn">知股</span>
      <span v-if="variant === 'horizontal' || variant === 'stacked'" class="zg-logo-en">Zhigu</span>
    </span>
  </span>
</template>
<script setup>
import symbol from '../../assets/brand/zhigu-symbol.svg?no-inline'
defineProps({
  variant: { type: String, default: 'stacked' },
  tone: { type: String, default: 'brand' }
})
</script>
<style scoped>
.zg-logo {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--zg-brand, #E85858);
  line-height: 1;
}
.zg-logo.is-ink { color: var(--zg-ink, #171717); }
.zg-logo.is-inverse { color: #fff; }
.zg-logo-mark {
  width: 32px;
  height: 28px;
  display: block;
  flex: none;
}
.is-nav .zg-logo-mark { width: 28px; height: 24px; }
.is-stacked {
  flex-direction: column;
  gap: 7px;
}
.is-stacked .zg-logo-mark { width: 30px; height: 27px; }
.is-stacked .zg-logo-word {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0;
}
.is-horizontal .zg-logo-word {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0;
}
.zg-logo-cn {
  font-family: var(--zg-font-editorial);
  font-weight: 600;
  font-size: 14px;
  color: var(--zg-ink, #171717);
}
.is-inverse .zg-logo-cn, .is-inverse .zg-logo-en { color: #fff; }
.is-nav .zg-logo-cn { font-size: 12px; }
.zg-logo-en {
  font-family: var(--zg-font-body);
  font-size: 10px;
  line-height: 14px;
  color: var(--zg-text-secondary, #666);
  font-weight: 400;
}
.is-ink .zg-logo-en,
.is-brand .zg-logo-en { color: var(--zg-text-secondary, #666); }
.is-hero { gap: 12px; }
.is-hero .zg-logo-mark { width: 36px; height: 32px; }
.is-hero .zg-logo-cn { font-size: 24px; letter-spacing: .06em; }
</style>
