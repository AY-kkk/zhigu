<template>
  <a class="zg-skip" href="#zg-main">跳到主要内容</a>
</template>
<style scoped>
.zg-skip {
  position: absolute;
  left: 12px;
  top: 12px;
  transform: translateY(calc(-100% - 24px));
  z-index: 500;
  padding: 8px 12px;
  background: var(--zg-ink);
  color: var(--zg-surface);
  border-radius: var(--zg-radius-control);
  text-decoration: none;
  font-size: 14px;
}
.zg-skip:focus {
  transform: none;
}
</style>
