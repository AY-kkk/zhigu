<template>
  <section class="hero" aria-labelledby="zg-hero-title">
    <div class="copy">
      <ZhiguLogo variant="hero" />
      <h1 id="zg-hero-title" aria-label="让投资观点，经得起验证"><span>让投资观点，</span><span>经得起验证</span></h1>
      <p class="lead">从一个观点出发，查看支持、反证与仍未确定的部分。</p>
    </div>
    <div class="visual" aria-hidden="true">
      <img
        :src="heroDesktop"
        :srcset="`${heroMobile} 768w, ${heroDesktop} 1280w`"
        sizes="(max-width: 767px) 1px, (max-width: 1023px) 640px, 380px"
        width="1280"
        height="853"
        alt=""
      >
    </div>
  </section>
</template>
<script setup>
import ZhiguLogo from './ZhiguLogo.vue'
import heroDesktop from '../../assets/brand/hero-research.webp'
import heroMobile from '../../assets/brand/hero-research-mobile.webp'
</script>
<style scoped>
.hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 32px;
  align-items: center;
  padding: 48px 0 32px;
}
.copy { min-width: 0; }
h1 {
  margin: 24px 0 16px;
  font-family: var(--zg-font-editorial);
  font-size: clamp(32px, 3vw, 40px);
  line-height: 1.35;
  font-weight: 600;
  color: var(--zg-ink);
}
h1 span { display: block; }
.lead {
  margin: 0;
  font-size: 16px;
  line-height: 28px;
  color: var(--zg-text-secondary);
  max-width: 20em;
}
.visual {
  aspect-ratio: 6 / 5;
  border-radius: 3px;
  overflow: hidden;
  background: var(--zg-surface-muted);
  box-shadow: 0 1px 0 rgba(23,23,23,.08);
}
.visual img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: 84% 50%;
  display: block;
}
@media (max-width: 1023px) {
  .hero {
    grid-template-columns: minmax(0, 1fr) minmax(0, .8fr);
    gap: 24px;
    padding: 36px 0 24px;
  }
  .visual { aspect-ratio: 1; }
  h1 { font-size: 32px; }
}
@media (max-width: 767px) {
  .hero { grid-template-columns: 1fr; padding: 28px 0 24px; }
  h1 { font-size: 32px; line-height: 44px; margin: 20px 0 12px; }
  .lead { font-size: 14px; line-height: 24px; max-width: 23em; }
  .visual { display: none; }
}
</style>
