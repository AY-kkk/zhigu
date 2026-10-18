<template>
  <section class="research-detail">
    <PageHeader
      :title="`期货研究 ${runId}`"
      :summary="run?.report?.question || '服务端任务状态、预算、证据和结构化报告。'"
      :status="displayState?.label || '读取中'"
      :primary-action="run?.report ? { label: '导出 HTML' } : null"
      :secondary-actions="canCancel ? [{ label: canceling ? '取消中…' : '取消研究' }] : []"
      @primary="download"
      @secondary="cancel"
    />

    <RequestError v-if="error" :error="error" @retry="load" />
    <LoadingState v-else-if="!run" label="正在从服务端恢复研究任务" :rows="5" />

    <template v-else>
      <div class="run-summary">
        <div class="status-line">
          <span class="system-status" :data-tone="displayState.tone">{{ displayState.label }}</span>
          <span>阶段：{{ run.stage || '未提供' }}</span>
          <span>已耗时：{{ elapsed }}</span>
          <span v-if="budget">模型 {{ budget.model_calls }} · 工具 {{ budget.tool_calls }} · Tokens {{ budget.input_tokens + budget.output_tokens }}</span>
        </div>
        <DataFreshness
          :as-of="run.asOf"
          :updated-at="run.createdAt"
          :stale="Boolean(run.failureCode)"
          :reason="run.failureCode"
        />
        <RequestError
          v-if="run.status === 'failed'"
          :error="{
            code: run.failureCode || 'FUTURES_RUN_FAILED',
            message: '研究失败；已完成阶段和诊断标识已保留。',
            retryable: false,
            traceId: run.traceId,
          }"
        />
      </div>

      <section v-if="run.report" class="report">
        <section>
          <h2>覆盖范围</h2>
          <p>{{ run.report.coverage_summary || '报告未提供覆盖摘要。' }}</p>
        </section>
        <section>
          <h2>结论</h2>
          <p class="report-body">{{ run.report.conclusion || '证据不足，不提供肯定性结论。' }}</p>
        </section>
        <section>
          <h2>主张核验</h2>
          <div class="table-scroll" tabindex="0">
            <table>
              <thead><tr><th>Claim</th><th>判定</th><th>说明</th></tr></thead>
              <tbody>
                <tr v-for="review in run.report.claim_reviews || []" :key="review.claim_id">
                  <td>{{ review.claim_id }}</td>
                  <td>{{ review.verdict }}</td>
                  <td>{{ review.explanation }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
        <section>
          <h2>反证</h2>
          <ul v-if="(run.report.counter_evidence || []).length">
            <li v-for="(item, index) in run.report.counter_evidence" :key="index">{{ item.text || item }}</li>
          </ul>
          <EmptyState v-else title="没有已登记反证" description="空白不代表不存在风险，只表示当前冻结证据中未登记。" />
        </section>
        <section>
          <h2>校验条件</h2>
          <ul v-if="(run.report.conditions || []).length">
            <li v-for="(item, index) in run.report.conditions" :key="index">
              {{ item.metric || item.condition }} {{ item.operator }} {{ item.threshold }}
              · {{ item.window || '未提供时间窗' }} · {{ item.source || '来源未提供' }}
            </li>
          </ul>
          <EmptyState v-else title="没有可执行校验条件" description="只有可读指标、运算符、阈值、时间窗和来源完整的条件才会显示。" />
        </section>
        <section>
          <h2>计算步骤</h2>
          <div class="table-scroll" tabindex="0">
            <table>
              <thead><tr><th>ID</th><th>公式版本</th><th>输入证据</th><th>结果</th></tr></thead>
              <tbody>
                <tr v-for="item in run.report.calculations || []" :key="item.id">
                  <td>{{ item.id }}</td>
                  <td>{{ item.formula_version }}</td>
                  <td>{{ (item.input_record_ids || []).join(', ') }}</td>
                  <td>{{ item.value ?? '不可计算' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
        <section>
          <h2>证据定位</h2>
          <div class="evidence-list">
            <button
              v-for="evidenceId in run.report.evidence_ids || []"
              :key="evidenceId"
              type="button"
              @click="openEvidence(evidenceId)"
            >
              {{ evidenceId }}
            </button>
          </div>
          <aside v-if="evidence" class="evidence-detail" aria-live="polite">
            <h3>{{ evidence.title || evidence.id }}</h3>
            <p>来源：{{ evidence.source || evidence.publisher || '未提供' }}</p>
            <p>定位：{{ evidence.locator || '未提供' }}</p>
            <p>时间：{{ evidence.published_at || evidence.as_of || '未提供' }}</p>
            <p>权限：{{ evidence.rights?.display ? '可展示' : '不可展示' }}</p>
            <blockquote>{{ evidence.quote || evidence.text || '未提供可展示片段' }}</blockquote>
          </aside>
        </section>
        <section>
          <h2>数据缺口与版本</h2>
          <ul><li v-for="gap in run.report.gaps || []" :key="gap">{{ gap }}</li></ul>
          <dl class="versions">
            <template v-for="(value, key) in run.report.versions || {}" :key="key">
              <dt>{{ key }}</dt><dd>{{ value || '未提供' }}</dd>
            </template>
          </dl>
        </section>
      </section>

      <section v-else class="pending-report">
        <h2>任务进度</h2>
        <p>报告只在服务端结构校验通过后显示。失败或取消会保留输入、已完成阶段和诊断标识。</p>
      </section>

      <div class="derived-actions">
        <button
          v-if="run.report?.id"
          type="button"
          @click="confirmHypothesis = true"
        >
          保存为假设
        </button>
      </div>
    </template>

    <ConfirmAction
      :open="confirmHypothesis"
      title="保存为可验证假设"
      impact="将把本次报告中的命题、证据关联和校验条件保存为新的假设草稿。不会自动激活为市场观点。"
      confirm-label="保存假设"
      :busy="savingHypothesis"
      @cancel="confirmHypothesis = false"
      @confirm="saveHypothesis"
    />
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ConfirmAction from '../../../components/system/ConfirmAction.vue'
import DataFreshness from '../../../components/system/DataFreshness.vue'
import EmptyState from '../../../components/system/EmptyState.vue'
import LoadingState from '../../../components/system/LoadingState.vue'
import RequestError from '../../../components/system/RequestError.vue'
import PageHeader from '../../../components/workspace/PageHeader.vue'
import {
  cancelRun,
  createHypothesis,
  exportRun,
  getEvidence,
  getRun,
} from '../api.js'
import { runDisplayState } from '../model.js'
import { useFuturesStore } from '../store.js'

const route = useRoute()
const router = useRouter()
const store = useFuturesStore()
const runId = computed(() => String(route.params.runId || ''))
const run = ref(null)
const error = ref(null)
const evidence = ref(null)
const canceling = ref(false)
const confirmHypothesis = ref(false)
const savingHypothesis = ref(false)
const generation = ref(0)
const accountGeneration = ref(store.generation)
let controller = null
let timer = null

const displayState = computed(() => runDisplayState(run.value || {}))
const canCancel = computed(() => ['queued', 'running', 'verifying'].includes(run.value?.status) && !canceling.value)
const budget = computed(() => run.value?.report?.usage || null)
const elapsed = computed(() => {
  if (!run.value?.createdAt) return '未提供'
  const start = Date.parse(run.value.createdAt)
  const end = run.value.displayState?.terminal ? Date.now() : Date.now()
  return `${Math.max(0, Math.round((end - start) / 1000))} 秒`
})

async function load() {
  clearTimeout(timer)
  controller?.abort()
  controller = new AbortController()
  const current = ++generation.value
  const requestedRunId = runId.value
  error.value = null
  try {
    const value = await getRun(requestedRunId, controller.signal)
    if (current !== generation.value || requestedRunId !== runId.value || accountGeneration.value !== store.generation) return
    run.value = value
    if (['queued', 'running', 'verifying', 'canceling'].includes(value.status)) timer = setTimeout(load, 3000)
  } catch (reason) {
    if (current === generation.value && reason?.code !== 'ERR_CANCELED') error.value = reason
  }
}

async function cancel() {
  canceling.value = true
  error.value = null
  try {
    await cancelRun(runId.value)
    await load()
  } catch (reason) {
    error.value = reason
  } finally {
    canceling.value = false
  }
}

async function download() {
  try {
    const response = await exportRun(runId.value)
    const url = URL.createObjectURL(response.data)
    const link = document.createElement('a')
    link.href = url
    link.download = `${runId.value}.html`
    link.rel = 'noopener'
    link.click()
    URL.revokeObjectURL(url)
  } catch (reason) {
    error.value = reason
  }
}

async function openEvidence(evidenceId) {
  try {
    evidence.value = await getEvidence(runId.value, evidenceId)
  } catch (reason) {
    error.value = reason
  }
}

async function saveHypothesis() {
  const report = run.value?.report
  if (!report?.id) return
  savingHypothesis.value = true
  try {
    const hypothesis = await createHypothesis({
      run_id: runId.value,
      report_id: report.id,
      claim_ids: (report.claim_reviews || []).map(item => item.claim_id),
      proposition: report.question,
      expires_at: report.horizon_end,
      conditions: report.conditions || [],
      activate: false,
    }, crypto.randomUUID())
    confirmHypothesis.value = false
    router.push(`/app/futures/hypotheses/${encodeURIComponent(hypothesis.id)}`)
  } catch (reason) {
    error.value = reason
  } finally {
    savingHypothesis.value = false
  }
}

watch(() => route.params.runId, load, { immediate: true })
watch(() => store.generation, () => {
  accountGeneration.value = store.generation
  run.value = null
  load()
})
onBeforeUnmount(() => {
  generation.value += 1
  clearTimeout(timer)
  controller?.abort()
})
</script>

<style scoped>
.run-summary { display: grid; gap: 12px; margin-bottom: 24px; padding: 18px; border: 1px solid var(--zg-line, #dedbd4); border-radius: 12px; }
.status-line { display: flex; flex-wrap: wrap; gap: 14px; align-items: center; }
.report { display: grid; gap: 26px; }
.report section { padding: 20px; border: 1px solid var(--zg-line, #dedbd4); border-radius: 12px; }
.report-body { font-size: 17px; line-height: 1.75; }
.table-scroll { max-width: 100%; overflow-x: auto; }
table { width: 100%; min-width: 620px; border-collapse: collapse; }
th, td { padding: 11px; text-align: left; border-bottom: 1px solid var(--zg-line, #dedbd4); }
.evidence-list { display: flex; flex-wrap: wrap; gap: 8px; }
.evidence-list button { min-height: 38px; padding: 7px 11px; border: 1px solid currentColor; border-radius: 999px; }
.evidence-detail { margin-top: 14px; padding: 16px; background: var(--zg-paper, #f7f5f0); }
.versions { display: grid; grid-template-columns: max-content 1fr; gap: 8px 16px; }
.derived-actions { margin-top: 22px; }
.derived-actions button { min-height: 42px; padding: 8px 14px; border: 1px solid currentColor; border-radius: 8px; }
</style>
