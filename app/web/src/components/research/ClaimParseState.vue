<template>
  <section class="parse" aria-busy="true">
    <div class="card">
      <span class="spinner" aria-hidden="true" />
      <div>
        <p class="title">正在解析你的投资观点…</p>
        <p class="sub">识别标的、研究期限与关键主张</p>
      </div>
    </div>
  </section>
</template>
<style scoped>
.card {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  padding: 24px;
  background: var(--zg-surface);
  border: 1px solid var(--zg-line);
  border-radius: var(--zg-radius-card);
}
.title { margin: 0; font-size: 16px; line-height: 24px; font-weight: 600; }
.sub { margin: 4px 0 0; font-size: 14px; line-height: 22px; color: var(--zg-text-secondary); }
.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid var(--zg-line);
  border-top-color: var(--zg-action);
  border-radius: 50%;
  animation: spin 900ms linear infinite;
  flex: none;
  margin-top: 2px;
}
@keyframes spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) {
  .spinner { animation: none; border-top-color: var(--zg-line); }
}
</style>
