<template>
  <section class="futures-page">
    <p class="eyebrow">假设版本跟踪</p><h2>研究假设</h2>
    <p v-if="error" class="error" role="alert">{{ error.message }}</p>
    <p v-if="actionError" class="error" role="alert">{{ actionError.message }}</p>
    <p v-else-if="!hypothesis">正在读取假设…</p>
    <template v-else>
      <section class="summary"><strong>{{ hypothesis.lifecycle }}</strong><span>版本 {{ hypothesis.version }}</span><span>用户判断：{{ hypothesis.user_view }}</span></section>
      <h3>{{ hypothesis.proposition }}</h3>
      <section><h4>验证与失效条件</h4><ul><li v-for="condition in hypothesis.conditions" :key="condition.id">{{ condition.role }}：{{ condition.instruction }}</li></ul></section>
      <section><h4>最新条件检查</h4><p v-if="!latestCheck">暂无可复核检查，首次决策需等待条件检查完成。</p><template v-else><p>检查版本 {{ latestCheck.version }}：{{ latestCheck.result }}</p><p>{{ latestCheck.reason }}</p><button v-if="hypothesis.reviewed_check_version !== latestCheck.version" @click="acknowledge">确认已复核</button></template></section>
      <section><h4>用户决策</h4><select v-model="view"><option value="retained">保留</option><option value="revised">修改</option><option value="rejected">否定</option></select><input v-model="reason" placeholder="决策理由"><button :disabled="!canDecide" @click="decide">记录决策</button></section>
      <section><h4>四维复盘</h4><label>事实兑现<input v-model="recap.facts"></label><label>传导支持<input v-model="recap.transmission"></label><label>实际合约表现<input v-model="recap.contract_performance"></label><label>数据充分性<input v-model="recap.data_sufficiency"></label><input v-model="recap.reason" placeholder="复盘理由"><button @click="saveRecap">保存复盘</button></section>
    </template>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { getChecks, getHypothesis, getRecap, patchHypothesis, putRecap } from '../api.js'

const route = useRoute()
const hypothesis = ref(null)
const checks = ref([])
const recapVersion = ref(1)
const error = ref(null)
const actionError = ref(null)
const view = ref('retained')
const reason = ref('')
const recap = ref({ facts: '', transmission: '', contract_performance: '', data_sufficiency: '', reason: '' })
const latestCheck = computed(() => checks.value[0] || null)
const canDecide = computed(() => Boolean(latestCheck.value && hypothesis.value?.reviewed_check_version === latestCheck.value.version && reason.value.trim()))

async function load() {
  try {
    const [value, checkPage, currentRecap] = await Promise.all([
      getHypothesis(route.params.id),
      getChecks(route.params.id, undefined, 100).catch(() => ({ items: [] })),
      getRecap(route.params.id).catch(() => null)
    ])
    hypothesis.value = value
    checks.value = checkPage.items || []
    if (currentRecap) {
      recapVersion.value = currentRecap.version
      recap.value = { facts: currentRecap.facts, transmission: currentRecap.transmission, contract_performance: currentRecap.contract_performance, data_sufficiency: currentRecap.data_sufficiency, reason: currentRecap.reason }
    }
  } catch (failure) { error.value = failure }
}

async function acknowledge() {
  actionError.value = null
  try {
    hypothesis.value = await patchHypothesis(route.params.id, { expected_version: hypothesis.value.version, action: 'acknowledge', reviewed_check_version: latestCheck.value.version })
  } catch (failure) { actionError.value = failure }
}

async function decide() {
  actionError.value = null
  try {
    hypothesis.value = await patchHypothesis(route.params.id, { expected_version: hypothesis.value.version, action: 'decide', user_view: view.value, reason: reason.value, reviewed_check_version: latestCheck.value.version })
  } catch (failure) { actionError.value = failure }
}

async function saveRecap() {
  actionError.value = null
  try {
    const saved = await putRecap(route.params.id, { hypothesis_id: route.params.id, version: recapVersion.value, ...recap.value })
    recapVersion.value = saved.version
    await load()
  } catch (failure) { actionError.value = failure }
}

onMounted(load)
</script>

<style scoped>
.eyebrow { color: var(--zg-action); }.summary { display: flex; gap: 16px; padding: 14px; border: 1px solid #ddd; }section { margin: 20px 0; }label { display: grid; gap: 4px; margin: 8px 0; }input,select { padding: 8px; }.error { color: #b42318; }
</style>
