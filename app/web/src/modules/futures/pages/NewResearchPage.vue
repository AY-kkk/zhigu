<template>
  <section class="futures-page">
    <h2>新建期货研究</h2>
    <form @submit.prevent="submitDraft">
      <label>品种<select v-model="input.product_id" required><option value="SHFE.CU">SHFE.CU</option></select></label>
      <label>研究合约<select v-model="input.contract_id" required><option disabled :value="null">请选择实际合约</option><option v-for="contract in contracts" :key="contract.id" :value="contract.id">{{ contract.id }}，最后交易日 {{ contract.last_trading_at }}</option></select></label>
      <label>研究期限<select v-model.number="input.horizon_days"><option :value="7">7 天</option><option :value="14">14 天</option><option :value="30">30 天</option></select></label>
      <label>观点文本<textarea v-model="input.text" minlength="20" maxlength="2000" placeholder="写下待验证的条件性观点"></textarea><small>{{ input.text.length }}/2000，至少 20 字</small></label>
      <label>材料<input type="file" accept=".pdf,.docx,.txt" @change="selectFile"><small>可选，单文件不超过 20 MiB</small></label>
      <button type="submit" :disabled="!valid || busy">{{ draft ? '重新解析修改后的输入' : '解析观点' }}</button>
    </form>
    <p v-if="error" class="error" role="alert">{{ error.message }}</p>
    <section v-if="draft">
      <h3>确认解析结果</h3>
      <p>草稿版本 {{ draft.revision }}，解析状态 {{ draft.parse_state }}</p>
      <label v-for="claim in draft.claims || []" :key="claim.id"><input type="checkbox" :value="claim" v-model="selectedClaims"> {{ claim.text }}</label>
      <button :disabled="selectedClaims.length === 0 || selectedClaims.length > 12 || busy || stale" @click="startResearch">确认并开始研究</button>
      <p v-if="stale" class="warning" role="alert">输入已修改，请重新解析并确认。</p>
    </section>
  </section>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { createDraft, createRun, getContracts, parseDraft, patchDraft, uploadDocument } from '../api.js'

const router = useRouter()
const input = ref({ product_id: 'SHFE.CU', contract_id: null, horizon_days: 14, text: '', document_id: null })
const contracts = ref([])
const file = ref(null)
const draft = ref(null)
const selectedClaims = ref([])
const busy = ref(false)
const stale = ref(false)
const error = ref(null)
const hydrating = ref(false)
const valid = computed(() => input.value.contract_id && (input.value.text.trim().length >= 20 || file.value))

async function loadContracts() {
  contracts.value = (await getContracts(input.value.product_id, undefined, 100)).items || []
}

function invalidateParsedInput() {
  if (hydrating.value) return
  stale.value = true
  selectedClaims.value = []
}

watch(input, invalidateParsedInput, { deep: true })
watch(file, invalidateParsedInput)

function selectFile(event) {
  file.value = event.target.files?.[0] || null
}

async function submitDraft() {
  busy.value = true
  error.value = null
  try {
    hydrating.value = true
    if (file.value) {
      const document = await uploadDocument(file.value)
      input.value.document_id = document.id
    }
    if (draft.value) {
      draft.value = await patchDraft(draft.value.id, draft.value.revision, input.value)
    } else {
      draft.value = await createDraft(input.value)
    }
    draft.value = await parseDraft(draft.value.id, draft.value.revision)
    selectedClaims.value = [...(draft.value.claims || [])]
    stale.value = false
  } catch (reason) { error.value = reason } finally { hydrating.value = false; busy.value = false }
}

async function startResearch() {
  if (stale.value) return
  busy.value = true
  error.value = null
  try {
    const run = await createRun({ draft_id: draft.value.id, expected_revision: draft.value.revision, claims: selectedClaims.value, accept_limits: true })
    router.push(`/app/futures/research/${run.id}`)
  } catch (reason) { error.value = reason } finally { busy.value = false }
}

onMounted(loadContracts)
</script>

<style scoped>
form { display: grid; gap: 16px; max-width: 720px; } label { display: grid; gap: 6px; } textarea { min-height: 160px; padding: 12px; } button { justify-self: start; padding: 10px 16px; background: var(--zg-action); color: white; border: 0; }.error { color: #b42318; }.warning { color: #9a6700; }
</style>
