<template>
  <section class="line" aria-label="证据关系摘要">
    <p v-if="model.partial" class="partial">部分发布，非完整覆盖</p>
    <div class="graphic" :class="{ unpublished: !model.published }" aria-hidden="true">
      <div class="arm support" :class="{ 'has-evidence': model.supportCount > 0 }">
        <span class="tick" />
      </div>
      <div class="node">
        <ZhiguIcon name="quote" :size="24" />
      </div>
      <div class="arm challenge" :class="{ 'has-evidence': model.challengeCount > 0 }">
        <span class="tick" />
      </div>
    </div>
    <div class="counts">
      <component
        :is="canJumpSupport ? 'button' : 'span'"
        type="button"
        class="count support"
        :class="{ link: canJumpSupport }"
        @click="canJumpSupport && $emit('jump', 'support')"
      >
        支持来源 {{ display(model.supportCount) }}
      </component>
      <component
        :is="canJumpChallenge ? 'button' : 'span'"
        type="button"
        class="count challenge"
        :class="{ link: canJumpChallenge }"
        @click="canJumpChallenge && $emit('jump', 'challenge')"
      >
        挑战来源 {{ display(model.challengeCount) }}
      </component>
      <span class="count unknown">未知项 {{ display(model.unknownCount) }}</span>
    </div>
    <p class="sr">{{ model.summary }}</p>
    <p v-if="model.published" class="note">{{ model.dualUse ? '同一来源可能关联不同论点；' : '' }}数量不代表证据强度。</p>
    <p v-if="!model.published" class="note">{{ model.summary }}</p>
  </section>
</template>
<script setup>
import { computed } from 'vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'
import { toEvidenceLineModel } from '../../utils/researchViewModel.js'

const props = defineProps({
  report: { type: Object, default: null }
})
defineEmits(['jump'])
const model = computed(() => toEvidenceLineModel(props.report))
const canJumpSupport = computed(() => model.value.published && model.value.supportCount > 0)
const canJumpChallenge = computed(() => model.value.published && model.value.challengeCount > 0)
function display(value) {
  return value == null ? '—' : String(value)
}
</script>
<style scoped>
.line { padding: 20px 24px; border: 1px solid var(--zg-line); border-radius: var(--zg-radius-card); background: var(--zg-surface); }
.graphic {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 0;
  margin: 4px 0 16px;
}
.arm {
  height: 1px;
  background: var(--zg-line);
  position: relative;
}
.arm.support { color: var(--zg-support-fg); }
.arm.challenge { color: var(--zg-challenge-fg); }
.tick { position: absolute; top: 50%; left: 45%; transform: translate(-50%, -50%); width: 11px; height: 11px; border: 1px solid var(--zg-control-border); border-radius: 50%; background: var(--zg-surface); }
.challenge .tick { left: 55%; }
.has-evidence .tick { background: currentColor; border-color: currentColor; box-shadow: 0 0 0 5px var(--zg-surface); }
.unpublished .node { color: var(--zg-ink); background: var(--zg-surface-muted); box-shadow: 0 0 0 6px var(--zg-paper); }
.node {
  width: 48px;
  height: 48px;
  border: 1px solid var(--zg-line);
  border-radius: 50%;
  display: grid;
  place-items: center;
  color: var(--zg-brand);
  background: var(--zg-support-bg);
  box-shadow: 0 0 0 6px var(--zg-paper);
}
.counts {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 16px;
}
.unknown { grid-column: 1 / -1; }
.count {
  border: 0;
  background: transparent;
  font: inherit;
  font-size: 14px;
  line-height: 22px;
  text-align: center;
  padding: 0;
  color: var(--zg-ink);
}
.count.support { color: var(--zg-support-fg); }
.count.challenge { color: var(--zg-challenge-fg); }
.count.unknown { color: var(--zg-text-secondary); }
.link { cursor: pointer; text-decoration: underline; text-underline-offset: 4px; text-decoration-color: transparent; min-height: 44px; }
.link:hover { text-decoration-color: currentColor; }
.note { margin: 12px 0 0; text-align: center; font-size: 12px; line-height: 18px; color: var(--zg-text-secondary); }
.partial { margin: 0 0 8px; font-size: 12px; color: var(--zg-warning-fg); }
.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
@media (max-width: 767px) { .line { padding: 20px 16px; } }
</style>
