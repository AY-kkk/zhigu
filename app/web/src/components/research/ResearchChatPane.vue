<template>
  <div class="pane" :class="{ 'is-pristine': isPristine }">
    <div class="scroll">
      <div class="zhigu-reading stack">
        <EditorialHero v-if="isPristine" />
        <p v-if="conversation.notice" class="notice">{{ conversation.notice.text }}</p>

        <blockquote v-if="claimText && !isPristine && !model?.report" class="claim">
          <p>{{ claimText }}</p>
        </blockquote>

        <ClaimParseState v-if="conversation.parseBusy" />
        <ResearchErrorState
          v-if="conversation.parseError && !conversation.parseBusy"
          title="解析失败"
          :message="conversation.parseError"
          retry-label="重新解析"
          @retry="conversation.parseCurrent"
        />
        <ResearchConfirmCard
          v-if="conversation.draft && !conversation.staleDraft && isNew"
          @confirmed="onConfirmed"
          @edit-claim="onEditClaim"
        />
        <p v-if="conversation.staleDraft && isNew" class="notice">原观点已修改，请重新解析后再确认。</p>

        <div v-if="isDetail && conversation.runLoading && !conversation.runView" class="zg-skeleton" aria-busy="true">
          <div class="zg-skeleton-line" />
          <div class="zg-skeleton-line" />
          <div class="zg-skeleton-line" />
        </div>
        <ResearchErrorState
          v-if="isDetail && conversation.runError && !conversation.runView"
          :title="inaccessible ? '该研究无法访问' : '暂时无法读取这项研究'"
          :message="conversation.runError"
          :retry-label="inaccessible ? '' : '重试'"
          @retry="reload"
        >
          <button v-if="inaccessible" type="button" class="zg-btn zg-btn-secondary" @click="router.push('/app/history')">返回历史</button>
        </ResearchErrorState>

        <template v-if="model">
          <header class="scope">
            <h1>{{ model.instrumentText || '观点研究' }}</h1>
            <template v-if="!model.report">
              <p>标的：{{ model.instrumentText }} · 期限：{{ emptyField(model.horizon) }}</p>
              <p>数据截止时间：{{ model.asOfText }} · {{ model.modeText }}</p>
            </template>
          </header>
          <ResearchStatus v-if="showStatus" />
          <EvidenceLine v-if="!model.report" :report="null" />
          <ResearchErrorState
            v-if="completedWithoutReport"
            title="报告暂不可用"
            message="研究已完成，但报告尚未可读取。"
            retry-label="重新查询"
            @retry="reload"
          />
          <Report
            v-if="model.report"
            :report="model.report"
            :claim="conversation.runView.claim"
            :instrument-id="model.instrumentId"
            :instrument-text="model.instrumentText"
            :horizon="model.horizon"
            @open-evidence="onOpenEvidence"
            @copy="onCopyReport"
            @reresearch="onReResearch"
          />
          <p v-if="model.report?.quality_status === 'incomplete'" class="notice">研究未完成，追问仅限已发布部分。</p>
        </template>

        <section v-if="conversation.followups.length" class="followups">
          <article v-for="item in conversation.followups" :key="item.questionId" class="follow">
            <p class="q">{{ item.text }}</p>
            <p v-if="!item.answer" class="wait">正在根据已发布证据作答…</p>
            <div v-else class="a">
              <p>{{ item.answer.answer }}</p>
              <p v-for="line in item.answer.limitations || []" :key="line" class="limit">{{ line }}</p>
              <p class="limit">仅使用本报告证据，不进行新检索。追问记录仅保留在本次会话，刷新后不会完整回放。</p>
              <p>
                <EvidenceReference
                  v-for="eid in allowedAnswerIds(item.answer)"
                  :key="eid"
                  :evidence-id="eid"
                  :allowed-ids="model?.allowedIds || []"
                  :index-map="model?.indexMap || {}"
                  @open="onOpenEvidence"
                />
              </p>
            </div>
          </article>
        </section>
      </div>
    </div>
    <div id="zg-composer" class="composer-wrap">
      <div v-if="isPristine" class="zhigu-reading examples">
        <button
          v-for="item in examples"
          :key="item.key"
          type="button"
          class="zg-btn zg-btn-secondary"
          @click="conversation.fillExample(item.text)"
        ><ZhiguIcon :name="item.key === 'fact' ? 'report' : item.key === 'challenge' ? 'search' : 'verify'" :size="16" />{{ item.label }}</button>
      </div>
      <ResearchComposer
        ref="composerRef"
        :model-value="conversation.draftText"
        :placeholder="composerMeta.placeholder"
        :hint="composerMeta.hint"
        :error="composerMeta.error"
        :action-label="composerMeta.label"
        :field-label="composerMeta.fieldLabel"
        :disabled="composerMeta.disabled"
        :busy="composerMeta.busy"
        :min-chars="composerMeta.minChars"
        :show-arrow="composerMeta.showArrow"
        :focus-token="conversation.composerFocusToken"
        @update:model-value="conversation.setDraftText"
        @submit="onSubmit"
      />
      <p v-if="isPristine" class="zhigu-reading coverage">从一条具体观点开始 · 支持已覆盖的 A 股与港股单公司研究</p>
    </div>
  </div>
</template>
<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Toast } from '@kousum/semi-ui-vue'
import { useResearchConversation } from '../../stores/researchConversation.js'
import { claimExamples, copyReportText, emptyField } from '../../utils/researchCopy.js'
import { toResearchViewModel } from '../../utils/researchViewModel.js'
import EditorialHero from '../brand/EditorialHero.vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'
import ClaimParseState from './ClaimParseState.vue'
import ResearchConfirmCard from './ResearchConfirmCard.vue'
import ResearchStatus from './ResearchStatus.vue'
import EvidenceLine from './EvidenceLine.vue'
import Report from './Report.vue'
import ResearchComposer from './ResearchComposer.vue'
import ResearchErrorState from './ResearchErrorState.vue'
import EvidenceReference from './EvidenceReference.vue'

const route = useRoute()
const router = useRouter()
const conversation = useResearchConversation()
const composerRef = ref(null)
const examples = computed(() => claimExamples(conversation.displayMode))

const isNew = computed(() => route.path === '/app/research/new')
const isDetail = computed(() => route.path.startsWith('/app/research/') && !!route.params.id)
const isPristine = computed(() => (
  isNew.value
  && !conversation.parseBusy
  && !conversation.draft
  && !conversation.userMessage
))
const claimText = computed(() => conversation.userMessage?.text || conversation.runView?.claim?.text || '')
const model = computed(() => toResearchViewModel(conversation.runView, conversation.instruments))
const showStatus = computed(() => {
  if (!conversation.runView) return false
  if (conversation.runView.report && conversation.runView.status === 'completed') return false
  return true
})
const completedWithoutReport = computed(() => conversation.runView?.status === 'completed' && !conversation.runView?.report)
const inaccessible = computed(() => conversation.runError === '该研究无法访问。')
const composerMeta = computed(() => {
  const mode = conversation.composerMode
  if (mode === 'busy') {
    return {
      disabled: true, label: '发送追问', showArrow: false, minChars: 1, fieldLabel: '追问',
      placeholder: '研究完成后，可基于已发布证据追问',
      hint: '',
      error: '', busy: false
    }
  }
  if (mode === 'followup') {
    return {
      disabled: false, label: '发送追问', showArrow: true, minChars: 1, fieldLabel: '追问',
      placeholder: '基于这份报告追问，不进行新检索',
      hint: '仅使用本报告证据，不进行新检索。每份报告最多 3 轮。追问记录仅保留在本次会话中，刷新后不会完整回放。',
      error: conversation.questionError, busy: conversation.questionBusy
    }
  }
  if (mode === 'followup_limit') {
    return {
      disabled: true, label: '发送追问', showArrow: false, minChars: 1, fieldLabel: '追问',
      placeholder: '已达到每份报告 3 轮追问上限',
      hint: '已达到每份报告 3 轮追问上限。如需新事实，请发起重新研究。',
      error: conversation.questionError, busy: false
    }
  }
  if (mode === 'locked') {
    return {
      disabled: true, label: '发送追问', showArrow: false, minChars: 1, fieldLabel: '追问',
      placeholder: '当前研究没有可追问的完整报告',
      hint: '',
      error: conversation.runError, busy: false
    }
  }
  return {
    disabled: false, label: '解析观点', showArrow: true, minChars: 20, fieldLabel: '投资观点',
    placeholder: '粘贴一个关于单家 A 股或港股公司的投资观点，20–2,000 字',
    hint: conversation.parseError ? '' : '',
    error: '', busy: conversation.parseBusy
  }
})

function allowedAnswerIds(answer) {
  const allowed = model.value?.allowedIds || []
  return (answer?.evidence_ids || []).filter((id) => allowed.includes(id))
}
function onConfirmed(id) {
  router.push(`/app/research/${id}`)
}
function onEditClaim() {
  composerRef.value?.focus()
}
function reload() {
  if (route.params.id) conversation.loadRun(route.params.id)
}
function onOpenEvidence(id, el) {
  conversation.openEvidence(id, el)
}
async function onSubmit() {
  composerRef.value?.markAttempted()
  const mode = conversation.composerMode
  if (mode === 'followup') {
    const text = conversation.draftText
    conversation.draftText = ''
    await conversation.askFollowup(text)
    return
  }
  if (mode === 'claim') await conversation.parseCurrent()
}
function onReResearch() {
  if (!conversation.currentRunId) return
  const parent = conversation.currentRunId
  conversation.resetAll()
  conversation.prepareNew({ parent_run_id: parent })
  router.push({ path: '/app/research/new', query: { parent_run_id: parent } })
}
async function onCopyReport() {
  const text = copyReportText({
    report: conversation.runView?.report,
    claim: conversation.runView?.claim,
    instrumentId: conversation.runView?.instrument_id,
    horizon: conversation.runView?.horizon
  })
  try {
    await navigator.clipboard.writeText(text || '')
    Toast.info({ content: '已复制', duration: 2 })
  } catch {
    Toast.error({ content: '复制失败' })
  }
}
</script>
<style scoped>
.pane {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.scroll {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
}
.is-pristine .scroll { flex: 0 0 auto; overflow: visible; }
.is-pristine { overflow: auto; }
.stack { padding-top: 28px; padding-bottom: 32px; display: grid; gap: 24px; }
.is-pristine .stack { padding-top: 0; padding-bottom: 0; }
.claim {
  margin: 0;
  padding: 16px 20px;
  border-left: 3px solid var(--zg-brand);
  background: var(--zg-surface);
}
.claim p { margin: 0; font-size: 16px; line-height: 28px; overflow-wrap: anywhere; }
.notice { margin: 0; color: var(--zg-text-secondary); font-size: 14px; line-height: 22px; }
.scope h1 { margin: 0 0 8px; font-family: var(--zg-font-editorial); font-size: 28px; line-height: 38px; font-weight: 600; }
.scope p { margin: 0; font-size: 12px; line-height: 18px; color: var(--zg-text-secondary); }
.composer-wrap { flex: none; border-top: 1px solid var(--zg-line); background: var(--zg-paper); }
.is-pristine .composer-wrap { border-top: 0; }
.examples { display: flex; flex-wrap: wrap; gap: 8px; padding-top: 0; padding-bottom: 4px; }
.examples .zg-btn { border-color: var(--zg-line); background: transparent; color: var(--zg-text-secondary); min-height: 36px; padding: 0 12px; }
.examples .zg-btn:hover { color: var(--zg-action); border-color: var(--zg-action); background: var(--zg-surface); }
.coverage { margin: 0 auto; padding-bottom: 32px; color: var(--zg-text-secondary); font-size: 12px; line-height: 20px; }
@media (max-width: 767px) {
  .stack { padding-top: 20px; gap: 20px; }
  .examples { gap: 6px; }
  .examples .zg-btn { padding: 0 8px; font-size: 12px; min-height: 44px; gap: 4px; }
}
.q { margin: 0; font-weight: 500; }
.a p, .wait, .limit { overflow-wrap: anywhere; }
.wait, .limit { color: var(--zg-text-secondary); font-size: 12px; line-height: 18px; }
.follow { display: grid; gap: 8px; padding: 16px 0; border-top: 1px solid var(--zg-line); }
</style>
