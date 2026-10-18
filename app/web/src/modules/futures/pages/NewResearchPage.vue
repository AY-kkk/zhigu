<template>
  <section class="new-research">
    <PageHeader
      title="发起期货研究"
      summary="在一个页面内完成问题与材料、解析核对、范围确认。提交前展示数据缺口、预算和阻断条件。"
    />

    <ModeBanner
      v-if="capabilities"
      :environment="capabilities.deploymentEnvironment"
      :provenance="capabilities.dataProvenance"
      :reason="capabilities.reason"
    />

    <ol class="research-steps" aria-label="研究创建步骤">
      <li v-for="(item, index) in steps" :key="item.label">
        <button
          type="button"
          :aria-current="stage === index + 1 ? 'step' : undefined"
          :disabled="index + 1 > highestStage"
          @click="stage = index + 1"
        >
          {{ index + 1 }}. {{ item.label }}
        </button>
      </li>
    </ol>

    <RequestError v-if="error" :error="error" @retry="retryLastAction" />

    <section v-if="stage === 1" aria-labelledby="stage-question">
      <h2 id="stage-question">问题与材料</h2>
      <form class="research-form" @submit.prevent="goParse">
        <label>
          品种
          <select v-model="input.product_id" required>
            <option disabled value="">请选择品种</option>
            <option v-for="product in products" :key="product.id" :value="product.productId">
              {{ product.exchange }} · {{ product.name }} · {{ product.productId }}
            </option>
          </select>
        </label>
        <label>
          研究合约
          <select v-model="input.contract_id" required>
            <option disabled :value="null">请选择具体合约</option>
            <option v-for="contract in contracts" :key="contract.id" :value="contract.id">
              {{ contract.id }} · 最后交易日 {{ contract.lastTradingAt }}
            </option>
          </select>
        </label>
        <label>
          观察期限
          <select v-model.number="input.horizon_days">
            <option :value="7">7 天</option>
            <option :value="14">14 天</option>
            <option :value="30">30 天</option>
          </select>
        </label>
        <label>
          条件性研究问题
          <textarea
            v-model="input.text"
            minlength="20"
            maxlength="2000"
            placeholder="描述可由来源、时间窗和数据条件验证的问题"
            required
          />
          <small>{{ input.text.length }}/2000，至少 20 字</small>
        </label>
        <FileUpload
          :files="materials"
          :disabled="busy"
          @select="selectMaterial"
          @cancel="cancelUpload"
          @retry="retryUpload"
        />
        <button class="primary" type="submit" :disabled="!questionValid || busy">
          解析并进入核对
        </button>
      </form>
    </section>

    <section v-else-if="stage === 2" aria-labelledby="stage-parse">
      <h2 id="stage-parse">解析与核对</h2>
      <p>草稿版本 {{ draft?.revision || '未创建' }} · 解析状态 {{ draft?.parse_state || '未开始' }}</p>
      <div v-if="claims.length" class="claim-list">
        <label v-for="claim in claims" :key="claim.id">
          <input v-model="selectedClaimIds" type="checkbox" :value="claim.id">
          <span>
            <strong>{{ claim.claim_type || claim.kind || 'claim' }}</strong>
            {{ claim.text }}
            <small>
              来源：{{ claim.source || claim.source_span_id || '未提供' }}
              · 时间：{{ claim.as_of || '未提供' }}
              · {{ claim.conflict ? '存在冲突' : '未标记冲突' }}
            </small>
          </span>
        </label>
      </div>
      <EmptyState
        v-else
        title="没有可核对的 claims"
        description="解析失败或没有抽取结果时，可重试单个材料，不需重传已经成功的材料。"
      />
      <div class="step-actions">
        <button type="button" @click="stage = 1">返回修改</button>
        <button type="button" class="primary" :disabled="!selectedClaimIds.length || selectedClaimIds.length > 12 || stale" @click="stage = 3">
          进入范围确认
        </button>
      </div>
    </section>

    <section v-else aria-labelledby="stage-scope">
      <h2 id="stage-scope">范围确认</h2>
      <div class="scope-grid">
        <section>
          <h3>研究输入</h3>
          <dl>
            <dt>品种</dt><dd>{{ input.product_id }}</dd>
            <dt>合约</dt><dd>{{ input.contract_id }}</dd>
            <dt>问题</dt><dd>{{ input.text }}</dd>
            <dt>Claims</dt><dd>{{ selectedClaimIds.length }} 条</dd>
          </dl>
        </section>
        <section>
          <h3>模型与预算</h3>
          <p>模型：{{ modelVersion || '由服务端配置' }}</p>
          <p>最大模型调用：{{ capabilities?.limits?.model_calls ?? '未提供' }}</p>
          <p>最大工具调用：{{ capabilities?.limits?.tool_calls ?? '未提供' }}</p>
          <p>Token 上限：{{ capabilities?.limits?.tokens ?? '未提供' }}</p>
        </section>
        <section>
          <h3>来源与缺口</h3>
          <p>允许来源：已登记、权限有效、截止时间满足条件的来源。</p>
          <p v-if="dataGaps.length" class="warning">数据缺口：{{ dataGaps.join('；') }}</p>
          <p v-else>当前输入未发现阻断性数据缺口。</p>
        </section>
      </div>
      <div class="step-actions">
        <button type="button" @click="stage = 2">返回核对</button>
        <button
          type="button"
          class="primary"
          :disabled="submitDisabled"
          @click="submitResearch(false)"
        >
          {{ submitBusy ? '正在提交…' : submitAttempted ? '使用同一幂等键重试' : '确认并开始研究' }}
        </button>
      </div>
    </section>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import EmptyState from '../../../components/system/EmptyState.vue'
import FileUpload from '../../../components/system/FileUpload.vue'
import ModeBanner from '../../../components/system/ModeBanner.vue'
import RequestError from '../../../components/system/RequestError.vue'
import PageHeader from '../../../components/workspace/PageHeader.vue'
import {
  createDraft,
  getClaims,
  getContracts,
  parseDraft,
  patchDraft,
  uploadDocument,
} from '../api.js'
import { canSubmitResearch, normalizeContracts } from '../model.js'
import { useFuturesStore } from '../store.js'

const route = useRoute()
const router = useRouter()
const store = useFuturesStore()
const steps = [
  { label: '问题与材料' },
  { label: '解析与核对' },
  { label: '范围确认' },
]
const input = ref({
  product_id: String(route.query.product_id || ''),
  contract_id: null,
  horizon_days: 14,
  text: '',
  document_id: null,
})
const contracts = ref([])
const materials = ref([])
const draft = ref(null)
const claims = ref([])
const selectedClaimIds = ref([])
const stage = ref(1)
const highestStage = ref(1)
const stale = ref(false)
const error = ref(null)
const busy = ref(false)
const submitBusy = ref(false)
const submitAttempted = ref(false)
const submitKey = ref('')
const uploadControllers = new Map()
const uploadKeys = new Map()
let contractController = null

const products = computed(() => store.products)
const capabilities = computed(() => store.capabilities)
const questionValid = computed(() => input.value.product_id && input.value.contract_id && input.value.text.trim().length >= 20)
const modelVersion = computed(() => draft.value?.versions?.model_config || '')
const dataGaps = computed(() => {
  const gaps = []
  if (!capabilities.value || capabilities.value.blocking) gaps.push(capabilities.value?.reason || '模块能力未就绪')
  if (!input.value.document_id && !input.value.text.trim()) gaps.push('缺少可解析材料')
  return gaps
})
const selectedClaims = computed(() => claims.value.filter(item => selectedClaimIds.value.includes(item.id)))
const submitGate = computed(() => canSubmitResearch({
  capabilities: capabilities.value,
  input: input.value,
  acceptedClaims: selectedClaims.value,
  parseState: draft.value?.parse_state,
}))
const submitDisabled = computed(() => submitBusy.value || !submitGate.value.allowed)

function invalidateParsedInput() {
  if (!draft.value) return
  stale.value = true
  highestStage.value = Math.min(highestStage.value, 2)
}

function selectMaterial({ file, rejected, reason }) {
  const id = crypto.randomUUID()
  materials.value = [{
    id,
    file,
    name: file.name,
    size: file.size,
    progress: 0,
    status: rejected ? 'failed' : 'queued',
    error: rejected ? reason : '',
  }]
  if (!rejected) uploadMaterial(id)
}

async function uploadMaterial(id, retry = false) {
  const item = materials.value.find(row => row.id === id)
  if (!item) return
  const key = retry || uploadKeys.get(id) || crypto.randomUUID()
  uploadKeys.set(id, key)
  const controller = new AbortController()
  uploadControllers.set(id, controller)
  item.status = 'uploading'
  item.error = ''
  try {
    const document = await uploadDocument(item.file, key, controller.signal, progress => { item.progress = progress })
    item.status = 'succeeded'
    item.progress = 100
    input.value.document_id = document.id
    uploadKeys.delete(id)
    invalidateParsedInput()
  } catch (reason) {
    item.status = 'failed'
    item.error = reason.message || '材料上传失败'
    error.value = reason
  } finally {
    uploadControllers.delete(id)
  }
}

function cancelUpload(id) {
  uploadControllers.get(id)?.abort()
  const item = materials.value.find(row => row.id === id)
  if (item) {
    item.status = 'failed'
    item.error = '已取消'
  }
}

function retryUpload(id) {
  error.value = null
  uploadMaterial(id, uploadKeys.get(id))
}

async function loadContracts() {
  contractController?.abort()
  contractController = new AbortController()
  if (!input.value.product_id) return
  const page = await getContracts(input.value.product_id, undefined, 100, contractController.signal)
  contracts.value = normalizeContracts(page)
}

async function goParse() {
  busy.value = true
  error.value = null
  try {
    if (!draft.value) {
      draft.value = await createDraft(input.value, crypto.randomUUID())
    } else {
      draft.value = await patchDraft(draft.value.id, {
        expected_revision: draft.value.revision,
        input: input.value,
      })
    }
    draft.value = await parseDraft(draft.value.id, draft.value.revision, crypto.randomUUID())
    const page = await getClaims(draft.value.id, undefined, 100)
    claims.value = page.items || []
    selectedClaimIds.value = claims.value.map(item => item.id)
    stale.value = false
    stage.value = 2
    highestStage.value = 2
  } catch (reason) {
    error.value = reason
  } finally {
    busy.value = false
  }
}

async function submitResearch(retry = false) {
  if (!retry && submitAttempted.value) {
    retry = true
  }
  if (!submitKey.value || !submitAttempted.value) submitKey.value = crypto.randomUUID()
  submitAttempted.value = true
  submitBusy.value = true
  error.value = null
  try {
    const run = await store.startResearch({
      draft_id: draft.value.id,
      expected_revision: draft.value.revision,
      claims: selectedClaims.value,
      accept_limits: true,
    }, retry, submitKey.value)
    router.push(`/app/futures/research/${encodeURIComponent(run.id)}`)
  } catch (reason) {
    error.value = reason
  } finally {
    submitBusy.value = false
  }
}

function retryLastAction() {
  if (stage.value === 3) submitResearch(true)
  else goParse()
}

watch(() => input.value.product_id, async () => {
  input.value.contract_id = null
  await loadContracts()
  invalidateParsedInput()
})
watch(input, invalidateParsedInput, { deep: true })

onMounted(async () => {
  await Promise.allSettled([store.loadCapabilities(), store.loadProducts()])
  await loadContracts()
})
onBeforeUnmount(() => {
  contractController?.abort()
  for (const controller of uploadControllers.values()) controller.abort()
})
</script>

<style scoped>
.research-steps { display: flex; gap: 8px; padding: 0; list-style: none; overflow-x: auto; }
.research-steps button { min-height: 42px; padding: 8px 14px; border: 1px solid var(--zg-line, #dedbd4); border-radius: 999px; }
.research-steps button[aria-current='step'] { border-color: var(--zg-action, #b4232c); color: var(--zg-action, #b4232c); }
.research-form { display: grid; gap: 16px; max-width: 780px; }
.research-form label { display: grid; gap: 6px; font-weight: 650; }
.research-form input, .research-form select, .research-form textarea { width: 100%; padding: 11px 12px; border: 1px solid var(--zg-line, #dedbd4); border-radius: 8px; font: inherit; }
.research-form textarea { min-height: 180px; resize: vertical; }
button.primary { background: var(--zg-action, #b4232c); color: white; border-color: var(--zg-action, #b4232c); }
.claim-list { display: grid; gap: 10px; }
.claim-list label { display: flex; gap: 12px; padding: 14px; border: 1px solid var(--zg-line, #dedbd4); }
.claim-list small { display: block; color: var(--zg-muted, #68616b); }
.scope-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
.scope-grid section { padding: 18px; border: 1px solid var(--zg-line, #dedbd4); }
.step-actions { display: flex; gap: 10px; margin-top: 22px; }
.step-actions button { min-height: 42px; padding: 8px 14px; border: 1px solid currentColor; border-radius: 8px; }
.warning { color: var(--zg-warning, #8a5600); }
@media (max-width: 767px) { .scope-grid { grid-template-columns: 1fr; } }
</style>
