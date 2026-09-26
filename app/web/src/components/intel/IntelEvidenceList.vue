<template>
  <section class="intel-evidence">
    <div class="intel-section-head">
      <div>
        <p class="eyebrow">Evidence</p>
        <h2>证据与出处</h2>
      </div>
      <span class="intel-muted">{{ items.length }} 条</span>
    </div>
    <div v-if="!items.length" class="intel-empty">暂无可展示证据。无依据时保持“待评估”，不推断结论。</div>
    <ol v-else class="intel-evidence-list">
      <li v-for="item in items" :key="item.evidence_id">
        <div class="intel-evidence-top">
          <span class="intel-pill" :class="`is-${item.grade}`">{{ gradeLabel(item.grade) }}</span>
          <span class="intel-pill is-soft">{{ stanceLabel(item.stance) }}</span>
          <span class="intel-muted">权重 {{ item.weight }}</span>
        </div>
        <p>{{ item.quote?.text || item.text || item.claim_key }}</p>
        <div class="intel-evidence-foot">
          <span>来源修订：{{ item.source_revision_id }}</span>
          <span>{{ item.status === 'active' ? '有效' : '历史/失效' }}</span>
        </div>
      </li>
    </ol>
  </section>
</template>
<script setup>
defineProps({ items: { type: Array, default: () => [] } })
const gradeLabel = (grade) => ({ fact: '事实', opinion: '观点', inference: '推测', rumor: '传闻' }[grade] || '证据')
const stanceLabel = (stance) => ({ support: '支持', refute: '反驳', context: '背景' }[stance] || '证据')
</script>
<style scoped>
.intel-section-head { display: flex; justify-content: space-between; align-items: end; gap: 12px; margin-bottom: 12px; }
.eyebrow { margin: 0 0 3px; color: var(--zg-action); font-size: 11px; font-weight: 700; letter-spacing: .12em; text-transform: uppercase; }
.intel-section-head h2 { margin: 0; font-size: 20px; }
.intel-muted { color: var(--zg-muted); font-size: 12px; }
.intel-empty { padding: 24px; border: 1px dashed var(--zg-border); border-radius: 14px; color: var(--zg-muted); }
.intel-evidence-list { display: grid; gap: 10px; padding: 0; margin: 0; list-style: none; }
.intel-evidence-list li { padding: 14px; border: 1px solid var(--zg-border); border-radius: 14px; background: var(--zg-surface); }
.intel-evidence-top, .intel-evidence-foot { display: flex; flex-wrap: wrap; gap: 7px; align-items: center; }
.intel-evidence-list p { margin: 10px 0; line-height: 1.7; }
.intel-evidence-foot { justify-content: space-between; color: var(--zg-muted); font-size: 11px; }
.intel-pill { padding: 3px 8px; border-radius: 999px; background: var(--zg-soft); color: var(--zg-muted); font-size: 11px; }
.intel-pill.is-fact { background: #e8f7ee; color: #18794e; }
.intel-pill.is-opinion { background: #eef2ff; color: #4457c7; }
.intel-pill.is-inference { background: #fff7df; color: #9a6700; }
.intel-pill.is-rumor { background: #fff0ee; color: #b42318; }
</style>
