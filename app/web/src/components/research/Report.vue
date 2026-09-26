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
    <section>
      <h2>核心判断</h2>
      <VerdictSummary :quality="report.quality_status" :verdict="report.verdict" :summary="report.summary" />
    </section>

    <section>
      <h2>事实核验</h2>
      <FactCheckTable
        :checks="report.fact_checks || []"
        :claim-items="claim?.items || []"
        :allowed-ids="allowedIds"
        :index-map="indexMap"
        @open="onOpen"
      />
    </section>

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
      <h2>逐条质疑</h2>
      <template v-if="(report.challenges || []).length">
        <article v-for="(item, i) in report.challenges" :key="'challenge' + i" class="challenge-item">
          <h3>{{ item.title || `反证 ${i + 1}` }}</h3>
          <p>{{ item.argument }}</p>
          <p class="small">
            <EvidenceReference
              v-for="eid in item.evidence_ids || []"
              :key="eid"
              :evidence-id="eid"
              :allowed-ids="allowedIds"
              :index-map="indexMap"
              @open="onOpen"
            />
          </p>
        </article>
      </template>
      <template v-else>
        <EvidenceArgument
          v-for="(item, i) in report.challenge || []"
          :key="'c' + i"
          :item="item"
          polarity="challenge"
          :allowed-ids="allowedIds"
          :index-map="indexMap"
          @open="onOpen"
        />
        <p v-if="!(report.challenge || []).length" class="empty">未找到有效反证。</p>
      </template>
    </section>

    <section>
      <h2>推理链缺口</h2>
      <ReasoningChainPanel :items="report.reasoning_gaps || []" />
    </section>

    <section>
      <h2>被忽略的风险</h2>
      <div v-if="(report.tail_risks || []).length" class="risks">
        <article v-for="(item, i) in report.tail_risks" :key="'risk' + i">
          <h3>{{ item.title }}</h3>
          <p>{{ item.description }}</p>
        </article>
      </div>
      <p v-else class="empty">未发现新的有效风险。</p>
    </section>

    <section>
      <h2>证实与证伪条件</h2>
      <div v-if="(report.test_conditions || []).length" class="conditions">
        <table>
          <thead><tr><th>主张</th><th>指标</th><th>证实方向</th><th>证伪方向</th><th>复核时间</th></tr></thead>
          <tbody>
            <tr v-for="item in report.test_conditions" :key="item.claim_id + item.metric">
              <td>{{ item.claim_id }}</td><td>{{ item.metric }}</td><td>{{ item.confirm_direction }}</td>
              <td>{{ item.falsify_direction }}</td><td>{{ item.review_at }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <ul v-else-if="(report.change_conditions || []).length">
        <li v-for="(item, i) in report.change_conditions" :key="'ch' + i">{{ item }}</li>
      </ul>
      <p v-else class="empty">本次已发布报告未提供该类条目</p>
    </section>

    <section>
      <h2>未知项与证据缺口</h2>
      <ul v-if="(report.unknowns || []).length">
        <li v-for="(item, i) in report.unknowns" :key="'u' + i">{{ item }}</li>
      </ul>
      <p v-else class="empty">本次已发布报告未提供该类条目</p>
    </section>

    <section>
      <h2>证据清单</h2>
      <div v-if="(report.evidence_index || []).length" class="evidence-index">
        <article v-for="(item, i) in report.evidence_index" :key="item.evidence_id">
          <p><strong>[{{ i + 1 }}] {{ item.title || item.evidence_id }}</strong></p>
          <p class="small">{{ item.source_grade }} · {{ item.verification_status === 'reported_only' ? '研报陈述，未独立核验' : '独立核验' }} · {{ item.locator }}</p>
          <EvidenceReference
            :evidence-id="item.evidence_id"
            :allowed-ids="allowedIds"
            :index-map="indexMap"
            @open="onOpen"
          />
        </article>
      </div>
      <p v-else class="empty">本次已发布报告未提供来源</p>
    </section>

    <div class="actions">
      <a class="zg-btn zg-btn-primary" :href="exportHref" download>下载 HTML 报告</a>
      <button type="button" class="zg-btn zg-btn-secondary" @click="$emit('copy')">复制报告</button>
      <button type="button" class="zg-btn zg-btn-secondary" @click="$emit('reresearch')">重新研究</button>
      <a class="zg-btn zg-btn-ghost" href="#zg-composer">基于该报告追问</a>
    </div>
    <p class="disclaimer">本报告仅供研究参考，不构成投资建议。</p>
  </article>
</template>
<script setup>
import { computed } from 'vue'
import FactCheckTable from './FactCheckTable.vue'
import ReasoningChainPanel from './ReasoningChainPanel.vue'
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
const exportHref = computed(() => `/api/finance/research/${encodeURIComponent(props.report.run_id || '')}/export?format=html`)
function onOpen(id, el) { emit('open-evidence', id, el) }
</script>
<style scoped>
.report { display: grid; gap: 30px; }
h2 { margin: 0 0 12px; font-family: var(--zg-font-editorial); font-size: 22px; line-height: 32px; font-weight: 600; }
h3 { margin: 0 0 4px; font-size: 16px; line-height: 26px; }
.claim, li, p { font-size: 16px; line-height: 28px; overflow-wrap: anywhere; }
.scope { padding-bottom: 24px; border-bottom: 1px solid var(--zg-line); }
.scope h2 { font-family: var(--zg-font-body); font-size: 13px; line-height: 20px; font-weight: 500; color: var(--zg-text-secondary); }
.scope .claim { margin: 0 0 20px; padding-left: 16px; border-left: 2px solid var(--zg-brand); font-family: var(--zg-font-editorial); font-size: 20px; line-height: 32px; }
.metadata { display: grid; grid-template-columns: 1fr 1fr; gap: 4px 20px; }
.report > section { scroll-margin-top: 24px; }
.report > section > h2 { display: flex; align-items: center; gap: 16px; }
.report > section > h2::after { content: ''; height: 1px; background: var(--zg-line); flex: 1; }
.meta { margin: 4px 0 0; font-size: 12px; line-height: 18px; color: var(--zg-text-secondary); }
.small { font-size: 13px; line-height: 20px; color: var(--zg-text-secondary); }
.notice { padding: 12px 16px; background: var(--zg-warning-bg); color: var(--zg-warning-fg); border-radius: var(--zg-radius-card); margin: 0; }
.empty { color: var(--zg-text-secondary); font-size: 14px; line-height: 22px; }
ul { padding-left: 18px; margin: 0; display: grid; gap: 8px; }
.challenge-item, .risks article, .evidence-index article { padding: 12px 0; border-bottom: 1px solid var(--zg-line); }
.conditions { overflow-x: auto; }
.conditions table { width: 100%; border-collapse: collapse; min-width: 680px; }
.conditions th, .conditions td { padding: 9px 8px; border-bottom: 1px solid var(--zg-line); text-align: left; vertical-align: top; font-size: 13px; line-height: 20px; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; }
.disclaimer { margin: 0; color: var(--zg-text-secondary); font-size: 13px; }
@media (max-width: 767px) { .metadata { grid-template-columns: 1fr; } .report { gap: 24px; } .scope .claim { font-size: 18px; line-height: 30px; } }
</style>
