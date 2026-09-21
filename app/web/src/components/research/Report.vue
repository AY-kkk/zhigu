<template>
  <article class="report">
    <header class="scope">
      <h2>原始观点与研究范围</h2>
      <p v-if="claim?.text" class="claim">{{ claim.text }}</p>
      <div class="metadata">
      <p class="meta">标的：{{ instrumentText }}</p>
      <p class="meta">期限：{{ horizon || claim?.horizon || '暂未提供' }}</p>
      <p class="meta">数据截止时间：{{ formatDateTime(report.as_of) }} · {{ modeText }}</p>
      <p class="meta">报告版本：{{ emptyField(report.version) }}</p>
      </div>
    </header>

    <p v-if="report.quality_status === 'incomplete'" class="notice">研究未完成。以下仅为已发布部分，不含总判断。</p>
    <p v-if="report.quality_status === 'incomplete' && report.summary" class="partial-summary">{{ report.summary }}</p>
    <VerdictSummary
      :quality="report.quality_status"
      :verdict="report.verdict"
      :summary="report.summary"
    />

    <EvidenceLine :report="report" @jump="jump" />

    <section id="zg-support">
      <h2>支持证据</h2>
      <EvidenceArgument
        v-for="(item, i) in report.support || []"
        :key="'s' + i"
        :item="item"
        polarity="support"
        :allowed-ids="allowedIds"
        :index-map="indexMap"
        @open="onOpen"
      />
      <p v-if="!(report.support || []).length" class="empty">本次已发布报告未提供支持证据</p>
    </section>

    <section id="zg-challenge">
      <h2>最强反证</h2>
      <EvidenceArgument
        v-for="(item, i) in report.challenge || []"
        :key="'c' + i"
        :item="item"
        polarity="challenge"
        :allowed-ids="allowedIds"
        :index-map="indexMap"
        @open="onOpen"
      />
      <p v-if="!(report.challenge || []).length" class="empty">本次已发布报告未提供反证</p>
    </section>

    <section>
      <h2>关键假设</h2>
      <ul v-if="(report.assumptions || []).length">
        <li v-for="(item, i) in report.assumptions" :key="'a' + i">{{ item }}</li>
      </ul>
      <p v-else class="empty">本次已发布报告未提供该类条目</p>
    </section>

    <section>
      <h2>改变判断的条件</h2>
      <ul v-if="(report.change_conditions || []).length">
        <li v-for="(item, i) in report.change_conditions" :key="'ch' + i">{{ item }}</li>
      </ul>
      <p v-else class="empty">本次已发布报告未提供该类条目</p>
    </section>

    <section id="zg-unknowns">
      <h2>未知项与证据缺口</h2>
      <ul v-if="(report.unknowns || []).length">
        <li v-for="(item, i) in report.unknowns" :key="'u' + i">{{ item }}</li>
      </ul>
      <p v-else class="empty">本次已发布报告未提供该类条目</p>
    </section>

    <section>
      <h2>来源索引</h2>
      <p v-if="allowedIds.length" class="index">
        <EvidenceReference
          v-for="eid in allowedIds"
          :key="'all' + eid"
          :evidence-id="eid"
          :allowed-ids="allowedIds"
          :index-map="indexMap"
          @open="onOpen"
        />
      </p>
      <p v-else class="empty">本次已发布报告未提供来源</p>
    </section>

    <div class="actions">
      <button type="button" class="zg-btn zg-btn-secondary" @click="$emit('copy')">复制报告</button>
      <button type="button" class="zg-btn zg-btn-secondary" @click="$emit('reresearch')">重新研究</button>
      <a class="zg-btn zg-btn-ghost" href="#zg-composer">基于该报告追问</a>
    </div>
  </article>
</template>
<script setup>
import { computed } from 'vue'
import EvidenceLine from './EvidenceLine.vue'
import EvidenceArgument from './EvidenceArgument.vue'
import EvidenceReference from './EvidenceReference.vue'
import VerdictSummary from './VerdictSummary.vue'
import { emptyField, formatDateTime, modeLabel } from '../../utils/researchCopy.js'
import { evidenceIndexMap } from '../../utils/researchViewModel.js'

const props = defineProps({
  report: { type: Object, required: true },
  claim: { type: Object, default: null },
  instrumentId: { type: String, default: '' },
  instrumentText: { type: String, default: '' },
  horizon: { type: String, default: '' }
})
const emit = defineEmits(['open-evidence', 'copy', 'reresearch'])
const allowedIds = computed(() => props.report.evidence_ids || [])
const indexMap = computed(() => evidenceIndexMap(props.report))
const modeText = computed(() => modeLabel(props.report.mode) || '暂未提供')
function onOpen(id, el) { emit('open-evidence', id, el) }
function jump(target) {
  const el = document.getElementById(target === 'support' ? 'zg-support' : 'zg-challenge')
  el?.scrollIntoView({ behavior: prefersReduce() ? 'auto' : 'smooth', block: 'start' })
}
function prefersReduce() {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
}
</script>
<style scoped>
.report { display: grid; gap: 32px; }
h2 {
  margin: 0 0 12px;
  font-family: var(--zg-font-editorial);
  font-size: 22px;
  line-height: 32px;
  font-weight: 600;
}
.claim, li { font-size: 16px; line-height: 28px; overflow-wrap: anywhere; }
.scope { padding-bottom: 24px; border-bottom: 1px solid var(--zg-line); }
.scope h2 { font-family: var(--zg-font-body); font-size: 13px; line-height: 20px; font-weight: 500; color: var(--zg-text-secondary); }
.scope .claim { margin: 0 0 20px; padding-left: 16px; border-left: 2px solid var(--zg-brand); font-family: var(--zg-font-editorial); font-size: 20px; line-height: 32px; }
.metadata { display: grid; grid-template-columns: 1fr 1fr; gap: 4px 20px; }
.report > section { scroll-margin-top: 24px; }
.report > section > h2 { display: flex; align-items: center; gap: 16px; }
.report > section > h2::after { content: ''; height: 1px; background: var(--zg-line); flex: 1; }
.partial-summary { font-size: 16px; line-height: 28px; margin: 0; }
.meta { margin: 4px 0 0; font-size: 12px; line-height: 18px; color: var(--zg-text-secondary); }
.notice {
  padding: 12px 16px;
  background: var(--zg-warning-bg);
  color: var(--zg-warning-fg);
  border-radius: var(--zg-radius-card);
  margin: 0;
}
.empty { color: var(--zg-text-secondary); font-size: 14px; line-height: 22px; }
ul { padding-left: 18px; margin: 0; display: grid; gap: 8px; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; }
@media (max-width: 767px) { .metadata { grid-template-columns: 1fr; } .report { gap: 24px; } .scope .claim { font-size: 18px; line-height: 30px; } }
</style>
