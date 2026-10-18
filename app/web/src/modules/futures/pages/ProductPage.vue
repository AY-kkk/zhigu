<template>
  <section class="product-workbench">
    <PageHeader
      :title="productTitle"
      :summary="productSummary"
      :primary-action="{ label: '发起研究', disabled: !coverageReady }"
      @primary="startResearch"
    >
      <template #status>
        <DataFreshness
          :as-of="workbench.data_as_of || ''"
          :updated-at="workbench.last_checked_at || ''"
          :stale="Boolean(workbench.stale)"
          :reason="workbench.stale_reason || ''"
        />
      </template>
    </PageHeader>

    <RequestError v-if="error" :error="error" @retry="load" />
    <LoadingState v-else-if="loading" label="正在读取品种事实与合约覆盖" :rows="5" />

    <template v-else>
      <section class="contract-section">
        <h2>合约口径</h2>
        <div class="contract-filter" role="group" aria-label="合约类型">
          <button
            v-for="item in contractKinds"
            :key="item.key"
            type="button"
            :aria-pressed="contractKind === item.key"
            @click="contractKind = item.key"
          >
            {{ item.label }}
          </button>
        </div>
        <div class="table-scroll" tabindex="0" aria-label="合约表">
          <table>
            <thead><tr><th>代码</th><th>口径</th><th>最后交易日</th><th>价格单位</th><th>乘数</th><th>来源版本</th></tr></thead>
            <tbody>
              <tr v-for="contract in visibleContracts" :key="contract.id">
                <td>{{ contract.id }}</td>
                <td>{{ contractKindLabel(contract.kind) }}</td>
                <td>{{ contract.lastTradingAt }}</td>
                <td>{{ contract.priceUnit }}</td>
                <td>{{ contract.multiplier }}</td>
                <td>{{ contract.sourceVersion }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <EmptyState
          v-if="!visibleContracts.length"
          title="没有该口径的可用合约"
          description="请切换口径，或查看来源覆盖是否仍满足连续/主力计算条件。"
        />
      </section>

      <div class="product-grid">
        <section><h2>行情</h2><p>{{ workbench.price_summary || '尚未登记行情摘要。' }}</p></section>
        <section><h2>期限结构</h2><p>{{ workbench.term_structure_summary || '缺少可比实际合约，暂不计算期限结构。' }}</p></section>
        <section><h2>持仓与库存</h2><p>{{ workbench.inventory_summary || '没有已登记且口径一致的持仓/库存指标。' }}</p></section>
        <section>
          <h2>来源覆盖</h2>
          <p v-if="coverageReady">已满足当前研究入口所需覆盖。</p>
          <RequestError
            v-else
            :error="{ code: 'SOURCE_COVERAGE_INSUFFICIENT', message: coverageReason, retryable: false, traceId: '' }"
          />
        </section>
      </div>
    </template>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DataFreshness from '../../../components/system/DataFreshness.vue'
import EmptyState from '../../../components/system/EmptyState.vue'
import LoadingState from '../../../components/system/LoadingState.vue'
import RequestError from '../../../components/system/RequestError.vue'
import PageHeader from '../../../components/workspace/PageHeader.vue'
import { getContracts, getWorkbench } from '../api.js'
import { normalizeContracts } from '../model.js'

const route = useRoute()
const router = useRouter()
const productId = String(route.params.productId || '')
const contracts = ref([])
const workbench = ref({})
const contractKind = ref('all')
const loading = ref(true)
const error = ref(null)
let controller = null

const contractKinds = [
  { key: 'all', label: '全部' },
  { key: 'dominant', label: '主力' },
  { key: 'continuous', label: '连续' },
  { key: 'actual', label: '具体合约' },
]
const productTitle = computed(() => workbench.value.product_name || productId)
const productSummary = computed(() => [
  workbench.value.exchange,
  productId,
  workbench.value.timezone || '交易时区未登记',
  workbench.value.product_kind,
].filter(Boolean).join(' · '))
const visibleContracts = computed(() => contractKind.value === 'all'
  ? contracts.value
  : contracts.value.filter(item => item.kind === contractKind.value))
const coverageReady = computed(() => Boolean(workbench.value.coverage_ready))
const coverageReason = computed(() => workbench.value.coverage_reason || '来源覆盖不足，不能提交不满足数据条件的研究。')

function contractKindLabel(kind) {
  return ({ dominant: '主力', continuous: '连续', actual: '具体合约' })[kind] || kind
}

async function load() {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  error.value = null
  try {
    const [contractPage, data] = await Promise.all([
      getContracts(productId, undefined, 100, controller.signal),
      getWorkbench(productId, { window: 60, price_type: 'settlement' }, controller.signal),
    ])
    contracts.value = normalizeContracts(contractPage)
    workbench.value = data || {}
  } catch (reason) {
    if (reason?.code !== 'ERR_CANCELED' && reason?.name !== 'CanceledError') error.value = reason
  } finally {
    loading.value = false
  }
}

function startResearch() {
  if (!coverageReady.value) return
  router.push({ path: '/app/futures/research/new', query: { product_id: productId } })
}

onMounted(load)
watch(() => route.params.productId, load)
onBeforeUnmount(() => controller?.abort())
</script>

<style scoped>
.contract-filter { display: flex; gap: 8px; margin: 12px 0; overflow-x: auto; }
.contract-filter button { min-height: 38px; padding: 7px 12px; border: 1px solid var(--zg-line, #dedbd4); border-radius: 999px; }
.contract-filter button[aria-pressed='true'] { border-color: var(--zg-action, #b4232c); color: var(--zg-action, #b4232c); }
.table-scroll { max-width: 100%; overflow-x: auto; border: 1px solid var(--zg-line, #dedbd4); border-radius: 10px; }
table { width: 100%; min-width: 760px; border-collapse: collapse; }
th, td { padding: 12px; text-align: left; border-bottom: 1px solid var(--zg-line, #dedbd4); }
.product-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px; margin-top: 22px; }
.product-grid section { padding: 20px; border: 1px solid var(--zg-line, #dedbd4); border-radius: 12px; }
@media (max-width: 767px) { .product-grid { grid-template-columns: 1fr; } }
</style>
