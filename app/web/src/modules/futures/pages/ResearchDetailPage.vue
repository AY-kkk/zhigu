<template>
  <section class="futures-page">
    <header class="page-header"><div><p class="eyebrow">冻结研究</p><h2>{{ runId }}</h2></div><button v-if="canCancel" @click="cancel">取消研究</button></header>
    <p v-if="error" class="error" role="alert">{{ error.message }}</p>
    <p v-else-if="!run">正在恢复研究任务…</p>
    <template v-else>
      <section class="run-state"><strong>{{ run.status }}</strong><span>{{ run.stage }}</span><span>数据截至 {{ run.as_of }}</span></section>
      <section v-if="run.report"><h3>质证报告</h3><p>结论：{{ run.report.conclusion }}</p><p>{{ run.report.coverage_summary }}</p><h4>主张核验</h4><ol><li v-for="review in run.report.claim_reviews" :key="review.claim_id">{{ review.verdict }}：{{ review.explanation }}</li></ol><h4>缺口</h4><ul><li v-for="gap in run.report.gaps" :key="gap">{{ gap }}</li></ul></section>
      <section v-else><h3>任务进度</h3><p>报告仅在服务端发布校验通过后出现；失败与取消保留输入和原因。</p></section>
      <div class="actions"><button v-if="run.report" @click="download">导出 HTML</button><button v-if="run.report" @click="saveHypothesis">保存为假设</button></div>
    </template>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { cancelRun, createHypothesis, exportRun, getRun } from '../api.js'
import { useFuturesStore } from '../store.js'

const route = useRoute()
const router = useRouter()
const store = useFuturesStore()
const runId = computed(() => route.params.runId)
const run = ref(null)
const error = ref(null)
const disposed = ref(false)
const generation = ref(0)
let timer = null
let controller = null
const canCancel = computed(() => ['queued', 'running', 'verifying'].includes(run.value?.status))

async function load() {
  clearTimeout(timer)
  controller?.abort()
  controller = new AbortController()
  const requestGeneration = ++generation.value
  const requestedRunId = runId.value
  run.value = null
  error.value = null
  try {
    const value = await getRun(requestedRunId, controller.signal)
    if (disposed.value || requestGeneration !== generation.value || requestedRunId !== route.params.runId || store.generation !== requestAccountGeneration) return
    run.value = value
    if (['queued', 'running', 'verifying', 'canceling'].includes(value.status)) timer = setTimeout(load, 2000)
  } catch (reason) {
    if (!disposed.value && requestGeneration === generation.value && reason.code !== 'ERR_CANCELED') error.value = reason
  }
}

let requestAccountGeneration = store.generation
async function cancel() { await cancelRun(runId.value); await load() }
async function download() {
  const response = await exportRun(runId.value)
  const url = URL.createObjectURL(response.data)
  const link = document.createElement('a'); link.href = url; link.download = `${runId.value}.html`; link.click(); URL.revokeObjectURL(url)
}
async function saveHypothesis() {
  const report = run.value.report
  const hypothesis = await createHypothesis({ run_id: runId.value, report_id: report.id, claim_ids: report.claim_reviews.map(item => item.claim_id), proposition: report.question, expires_at: report.horizon_end, conditions: report.conditions, activate: true })
  router.push(`/app/futures/hypotheses/${hypothesis.id}`)
}

watch(() => route.params.runId, load, { immediate: true })
watch(() => store.generation, () => {
  requestAccountGeneration = store.generation
  run.value = null
  load()
})
onBeforeUnmount(() => {
  disposed.value = true
  generation.value += 1
  clearTimeout(timer)
  controller?.abort()
})
</script>

<style scoped>
.page-header { display: flex; justify-content: space-between; align-items: center; }.eyebrow { margin: 0; color: var(--zg-action); }.run-state { display: flex; gap: 16px; padding: 16px; border: 1px solid #ddd; }.actions { display: flex; gap: 8px; }.error { color: #b42318; }
</style>
