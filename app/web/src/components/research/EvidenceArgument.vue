<template>
  <article class="arg" :class="polarity">
    <header>
      <EvidenceTag :kind="item.claim_type" />
      <EvidenceTag v-if="polarity === 'support'" kind="supported" label="支持证据" />
      <EvidenceTag v-else-if="polarity === 'challenge'" kind="challenged" label="反方证据" />
    </header>
    <p>{{ item.text }}</p>
    <p class="refs">
      <EvidenceReference
        v-for="eid in item.evidence_ids || []"
        :key="eid"
        :evidence-id="eid"
        :allowed-ids="allowedIds"
        :index-map="indexMap"
        :title="titles[eid] || ''"
        @open="forward"
      />
    </p>
  </article>
</template>
<script setup>
import EvidenceTag from './EvidenceTag.vue'
import EvidenceReference from './EvidenceReference.vue'

defineProps({
  item: { type: Object, required: true },
  polarity: { type: String, default: 'support' },
  allowedIds: { type: Array, default: () => [] },
  indexMap: { type: Object, default: () => ({}) },
  titles: { type: Object, default: () => ({}) }
})
const emit = defineEmits(['open'])
function forward(id, el) { emit('open', id, el) }
</script>
<style scoped>
.arg {
  margin-top: 12px;
  padding: 24px;
  border-radius: var(--zg-radius-card);
  background: var(--zg-surface);
  border: 1px solid var(--zg-line);
  border-left-width: 3px;
}
.support { border-left-color: var(--zg-support-fg); }
.challenge { border-left-color: var(--zg-challenge-fg); }
header { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 8px; }
p { margin: 0; font-size: 16px; line-height: 28px; color: var(--zg-ink); overflow-wrap: anywhere; }
.refs { margin-top: 8px; }
@media (max-width: 767px) { .arg { padding: 20px 16px; } }
</style>
