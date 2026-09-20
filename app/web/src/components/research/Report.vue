<template>
  <article class="report">
    <header class="scope">
      <TypographyTitle heading="5">原始观点与研究范围</TypographyTitle>
      <TypographyParagraph v-if="claim?.text">{{ claim.text }}</TypographyParagraph>
      <p>公司：{{ instrumentId || '未标注' }} · 期限：{{ horizon || claim?.horizon || '未标注' }}</p>
      <p>数据截止时间：{{ formatDateTime(report.as_of) }} · {{ modeLabel(report.mode) || '暂未提供' }}</p>
    </header>
    <Tag :color="verdictColor">{{ qualityLabel }}{{ verdictText ? ` · ${verdictText}` : '' }}</Tag>
    <TypographyParagraph v-if="report.summary" class="summary">{{ report.summary }}</TypographyParagraph>
    <Divider />
    <section>
      <TypographyTitle heading="5">支持证据</TypographyTitle>
      <ul>
        <li v-for="(item, i) in report.support || []" :key="'s' + i">
          {{ item.text }}
          <EvidenceReference
            v-for="eid in item.evidence_ids || []"
            :key="'s' + eid"
            :evidence-id="eid"
            :allowed-ids="allowedIds"
            :index-map="indexMap"
            :title="sourceTitle(eid)"
            @open="onOpen"
          />
        </li>
      </ul>
    </section>
    <section>
      <TypographyTitle heading="5">最强反证</TypographyTitle>
      <ul>
        <li v-for="(item, i) in report.challenge || []" :key="'c' + i">
          {{ item.text }}
          <EvidenceReference
            v-for="eid in item.evidence_ids || []"
            :key="'c' + eid"
            :evidence-id="eid"
            :allowed-ids="allowedIds"
            :index-map="indexMap"
            :title="sourceTitle(eid)"
            @open="onOpen"
          />
        </li>
      </ul>
    </section>
    <section>
      <TypographyTitle heading="5">改变判断的条件</TypographyTitle>
      <ul>
        <li v-for="(item, i) in report.change_conditions || []" :key="'ch' + i">{{ item }}</li>
      </ul>
    </section>
    <section>
      <TypographyTitle heading="5">未知项与证据缺口</TypographyTitle>
      <ul>
        <li v-for="(item, i) in report.unknowns || []" :key="'u' + i">{{ item }}</li>
      </ul>
    </section>
    <section v-if="allowedIds.length">
      <TypographyTitle heading="5">来源汇总</TypographyTitle>
      <p>
        <EvidenceReference
          v-for="eid in allowedIds"
          :key="'all' + eid"
          :evidence-id="eid"
          :allowed-ids="allowedIds"
          :index-map="indexMap"
          @open="onOpen"
        />
      </p>
    </section>
  </article>
</template>
<script setup>
import { computed } from 'vue'
import { Divider, Tag, TypographyParagraph, TypographyTitle } from '@kousum/semi-ui-vue'
import EvidenceReference from './EvidenceReference.vue'
import { formatDateTime, modeLabel, VERDICT_LABELS } from '../../utils/researchCopy.js'

const props = defineProps({
  report: { type: Object, required: true },
  claim: { type: Object, default: null },
  instrumentId: { type: String, default: '' },
  horizon: { type: String, default: '' }
})
const emit = defineEmits(['open-evidence'])
const allowedIds = computed(() => props.report.evidence_ids || [])
const indexMap = computed(() => {
  const map = {}
  allowedIds.value.forEach((id, i) => { map[id] = i + 1 })
  return map
})
const qualityLabel = computed(() => {
  if (props.report.quality_status === 'incomplete') return '研究未完成'
  if (props.report.quality_status === 'failed') return '研究失败'
  return '研究完成'
})
const verdictText = computed(() => VERDICT_LABELS[props.report.verdict] || '')
const verdictColor = computed(() => {
  if (props.report.verdict === 'supported') return 'green'
  if (props.report.verdict === 'challenged' || props.report.verdict === 'insufficient') return 'red'
  return 'grey'
})
function sourceTitle() { return '查看证据来源' }
function onOpen(id, el) { emit('open-evidence', id, el) }
</script>
<style scoped>
.report { display: grid; gap: 16px; max-width: 100%; }
.scope p, .summary, li { overflow-wrap: anywhere; font-size: 16px; line-height: 28px; }
ul { padding-left: 18px; margin: 8px 0 0; }
</style>
