<template>
  <section class="futures-page">
    <header class="page-header">
      <div><p class="eyebrow">品种工作台</p><h2>{{ productId }}</h2></div>
      <label>实际合约<select v-model="contractId"><option value="">全部实际合约</option><option v-for="item in contracts" :key="item.id" :value="item.id">{{ item.id }}</option></select></label>
      <label>窗口<select v-model.number="window"><option :value="20">20 个交易日</option><option :value="60">60 个交易日</option><option :value="120">120 个交易日</option></select></label>
    </header>
    <div class="tabs" role="tablist">
      <button v-for="item in tabs" :key="item.key" role="tab" :aria-selected="tab === item.key" @click="tab = item.key">{{ item.label }}</button>
    </div>
    <p v-if="error" class="error" role="alert">{{ error.message || '工作台读取失败' }}</p>
    <p v-else-if="loading">正在读取冻结数据覆盖…</p>
    <template v-else>
      <section v-if="tab === 'overview'"><h3>研究概览</h3><p>数据截至：{{ workbench.data_as_of || '尚未登记' }}</p><p>最后检查：{{ workbench.last_checked_at || '尚未检查' }}</p></section>
      <section v-else-if="tab === 'industry'"><h3>产业与供需</h3><ul><li v-for="gap in workbench.missing_metrics || []" :key="gap">{{ gap }}</li><li v-if="!(workbench.missing_metrics || []).length">没有已登记的产业指标缺口</li></ul></section>
      <section v-else-if="tab === 'structure'"><h3>期现结构</h3><p>只使用实际合约；跨日、跨单位或不可比口径不计算。</p></section>
      <section v-else-if="tab === 'verify'"><h3>观点验证</h3><RouterLink class="primary" to="/app/futures/research/new">进入新建研究</RouterLink></section>
      <section v-else><h3>跟踪复盘</h3><p>假设只从已发布研究报告保存，不自动生成观点。</p></section>
    </template>
  </section>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { getContracts, getWorkbench } from '../api.js'

const route = useRoute()
const productId = route.params.productId
const contractId = ref('')
const window = ref(60)
const tab = ref('overview')
const tabs = [
  { key: 'overview', label: '研究概览' }, { key: 'industry', label: '产业与供需' },
  { key: 'structure', label: '期现结构' }, { key: 'verify', label: '观点验证' }, { key: 'tracking', label: '跟踪复盘' }
]
const contracts = ref([])
const workbench = ref({})
const loading = ref(false)
const error = ref(null)
let controller

async function load() {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  error.value = null
  try {
    const [contractPage, data] = await Promise.all([
      getContracts(productId, undefined, 100, controller.signal),
      getWorkbench(productId, { contract_id: contractId.value || undefined, window: window.value, price_type: 'settlement' }, controller.signal)
    ])
    contracts.value = contractPage.items || []
    workbench.value = data
  } catch (reason) {
    if (reason.code !== 'ERR_CANCELED') error.value = reason
  } finally { loading.value = false }
}

onMounted(load)
watch([contractId, window], load)
onBeforeUnmount(() => controller?.abort())
</script>

<style scoped>
.page-header { display: flex; gap: 16px; align-items: end; flex-wrap: wrap; }.page-header h2 { margin: 0; }.eyebrow { margin: 0; color: var(--zg-muted); }
.tabs { display: flex; overflow-x: auto; border-bottom: 1px solid #ddd; margin: 20px 0; }.tabs button { border: 0; background: transparent; padding: 12px; }.tabs button[aria-selected="true"] { color: var(--zg-action); border-bottom: 2px solid currentColor; }
label { display: grid; gap: 4px; font-size: 13px; } select { min-width: 160px; padding: 8px; } .error { color: #b42318; }.primary { display: inline-block; padding: 10px 14px; color: white; background: var(--zg-action); }
</style>
