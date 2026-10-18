<template>
  <section class="futures-overview">
    <PageHeader
      title="期货研究概览"
      summary="先确认运行模式、数据状态和待处理事项，再进入品种、研究或假设详情。"
      :primary-action="{ label: '发起期货研究', disabled: store.capabilities?.blocking }"
      @primary="$router.push('/app/futures/research/new')"
    >
      <template #status>
        <ModeBanner
          v-if="store.capabilities"
          :environment="store.capabilities.deploymentEnvironment"
          :provenance="store.capabilities.dataProvenance"
          :reason="store.capabilities.reason"
        />
      </template>
    </PageHeader>

    <RequestError
      v-if="store.error"
      :error="store.error"
      @retry="load"
    />
    <LoadingState v-else-if="loading" label="正在读取期货能力与摘要" :rows="4" />

    <div v-else class="overview-grid">
      <section class="overview-card capability">
        <h2>模式与阻断能力</h2>
        <CapabilityStatus :state="store.capabilities" />
      </section>

      <section class="overview-card freshness">
        <h2>数据新鲜度与待处理事项</h2>
        <DataFreshness
          :as-of="latestAsOf"
          :updated-at="latestUpdatedAt"
          :stale="Boolean(latestFailure)"
          :reason="latestFailure"
        />
        <RouterLink to="/app/futures/notifications">
          {{ unreadNotifications.length }} 条未读通知
        </RouterLink>
      </section>

      <section class="overview-card watchlist">
        <h2>关注品种与可用合约</h2>
        <ul v-if="products.length">
          <li v-for="product in products" :key="product.id">
            <RouterLink :to="`/app/futures/products/${encodeURIComponent(product.productId)}`">
              {{ product.name }} · {{ product.exchange }} · {{ product.productId }}
            </RouterLink>
          </li>
        </ul>
        <EmptyState
          v-else
          title="尚未登记可研究品种"
          description="只有通过来源和准入检查的品种会出现在这里。"
        />
      </section>

      <section class="overview-card runs">
        <h2>最近研究运行</h2>
        <ul v-if="recentRuns.length">
          <li v-for="run in recentRuns" :key="run.id">
            <RouterLink :to="`/app/futures/research/${encodeURIComponent(run.id)}`">
              {{ run.id }} · {{ run.status }} · {{ run.asOf }}
            </RouterLink>
          </li>
        </ul>
        <EmptyState
          v-else
          title="还没有研究运行"
          description="发起研究后，这里会显示服务端状态、时间和结果入口。"
        />
      </section>

      <section class="overview-card hypotheses">
        <h2>活跃假设与临近校验条件</h2>
        <ul v-if="activeHypotheses.length">
          <li v-for="item in activeHypotheses" :key="item.id">
            <RouterLink :to="`/app/futures/hypotheses/${encodeURIComponent(item.id)}`">
              {{ item.proposition }} · {{ item.lifecycle }} · {{ item.expiresAt }}
            </RouterLink>
          </li>
        </ul>
        <EmptyState
          v-else
          title="没有待验证假设"
          description="假设只从已发布研究报告保存，不会自动生成市场观点。"
        />
      </section>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import CapabilityStatus from '../../../components/system/CapabilityStatus.vue'
import DataFreshness from '../../../components/system/DataFreshness.vue'
import EmptyState from '../../../components/system/EmptyState.vue'
import LoadingState from '../../../components/system/LoadingState.vue'
import ModeBanner from '../../../components/system/ModeBanner.vue'
import RequestError from '../../../components/system/RequestError.vue'
import PageHeader from '../../../components/workspace/PageHeader.vue'
import { useFuturesStore } from '../store.js'

const store = useFuturesStore()
const loading = ref(true)

const products = computed(() => store.products)
const recentRuns = computed(() => (store.resources.runs || store.runs || []).slice(0, 5))
const activeHypotheses = computed(() => (store.resources.hypotheses || store.hypotheses || [])
  .filter(item => !['closed', 'deleted', 'expired'].includes(item.lifecycle))
  .slice(0, 6))
const unreadNotifications = computed(() => (store.resources.notifications || store.notifications || [])
  .filter(item => !item.readAt && !item.read_at))
const latestAsOf = computed(() => recentRuns.value[0]?.asOf || '')
const latestUpdatedAt = computed(() => recentRuns.value[0]?.createdAt || '')
const latestFailure = computed(() => recentRuns.value.find(item => item.status === 'failed')?.failureCode || '')

async function load() {
  loading.value = true
  await Promise.allSettled([
    store.loadCapabilities(),
    store.loadProducts(),
    store.loadRuns(),
    store.loadHypotheses(),
    store.loadNotifications({ read: false }),
  ])
  loading.value = false
}

onMounted(load)
</script>

<style scoped>
.overview-grid {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  gap: 18px;
}
.overview-card {
  grid-column: span 4;
  min-width: 0;
  padding: 20px;
  border: 1px solid var(--zg-line, #dedbd4);
  border-radius: 12px;
  background: var(--zg-surface, #fffefb);
}
.overview-card.capability { grid-column: span 5; }
.overview-card.freshness { grid-column: span 7; }
.overview-card h2 { margin: 0 0 14px; font-size: 18px; }
.overview-card ul { display: grid; gap: 10px; margin: 0; padding-left: 18px; }
@media (max-width: 1279px) {
  .overview-card, .overview-card.capability, .overview-card.freshness { grid-column: span 6; }
}
@media (max-width: 767px) {
  .overview-grid { display: block; }
  .overview-card { margin-bottom: 14px; }
}
</style>
