<template>
  <div class="fact-check-table">
    <table>
      <thead>
        <tr><th>主张</th><th>状态</th><th>原因</th><th>证据</th></tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="row.claim_id">
          <td>{{ row.text }}</td>
          <td><EvidenceTag :kind="statusKind(row.status)" :label="statusLabel(row.status)" /></td>
          <td>{{ row.reason }}</td>
          <td>
            <EvidenceReference
              v-for="id in row.evidence_ids || []"
              :key="id"
              :evidence-id="id"
              :allowed-ids="allowedIds"
              :index-map="indexMap"
              @open="$emit('open', $event)"
            />
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
<script setup>
import { computed } from 'vue'
import EvidenceTag from './EvidenceTag.vue'
import EvidenceReference from './EvidenceReference.vue'

const props = defineProps({
  checks: { type: Array, default: () => [] },
  claimItems: { type: Array, default: () => [] },
  allowedIds: { type: Array, default: () => [] },
  indexMap: { type: Object, default: () => ({}) }
})
defineEmits(['open'])
const rows = computed(() => {
  const byId = new Map((props.checks || []).map((row) => [row.claim_id, row]))
  return (props.claimItems || []).map((item) => ({
    text: item.text,
    claim_id: item.claim_id,
    status: byId.get(item.claim_id)?.status || 'uncertain',
    reason: byId.get(item.claim_id)?.reason || '未提供核验结果',
    evidence_ids: byId.get(item.claim_id)?.evidence_ids || []
  }))
})
function statusKind(status) {
  return { supported: 'fact', contradicted: 'challenged', prerequisite_missing: 'assumption', uncertain: 'unknown' }[status] || 'unknown'
}
function statusLabel(status) {
  return { supported: '成立', contradicted: '不成立', prerequisite_missing: '被前置', uncertain: '存疑' }[status] || status
}
</script>
<style scoped>
.fact-check-table { overflow-x: auto; }
table { width: 100%; border-collapse: collapse; min-width: 620px; }
th, td { padding: 10px 8px; border-bottom: 1px solid var(--zg-line); text-align: left; vertical-align: top; font-size: 13px; line-height: 20px; }
th { color: var(--zg-text-secondary); font-weight: 600; }
</style>
