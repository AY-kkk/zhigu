<template>
  <div class="market">
    <StrategySubnav />
    <header class="head">
      <h2>策略市场</h2>
      <p class="hint">平台整理与审核的完整规则目录。复制后成为你的私有草稿，可修改并回测；历史证据只属于原记录。</p>
    </header>

    <div class="filters">
      <input
        v-model="store.filters.q"
        class="search"
        type="search"
        placeholder="搜索策略名称、标签"
        aria-label="搜索策略"
        @input="onSearchInput"
      >
      <select aria-label="策略思路" :value="store.filters.category" @change="apply('category', $event)">
        <option value="">全部思路</option>
        <option v-for="c in categoryOptions" :key="c" :value="c">{{ c }}</option>
      </select>
      <select aria-label="执行周期" :value="store.filters.period" @change="apply('period', $event)">
        <option value="">全部周期</option>
        <option v-for="p in periodOptions" :key="p" :value="p">{{ p }}</option>
      </select>
      <select aria-label="支持市场" :value="store.filters.market" @change="apply('market', $event)">
        <option value="">全部市场</option>
        <option value="A">A股</option>
        <option value="HK">港股</option>
      </select>
      <select aria-label="规则校验状态" :value="store.filters.validation" @change="apply('validation', $event)">
        <option value="">全部校验状态</option>
        <option value="passed">已通过</option>
        <option value="pending">未校验</option>
        <option value="failed">未通过</option>
      </select>
      <select aria-label="回测证据" :value="store.filters.evidence" @change="apply('evidence', $event)">
        <option value="">全部证据状态</option>
        <option value="has_evidence">有历史模拟记录</option>
        <option value="not_tested">未回测</option>
      </select>
    </div>

    <p v-if="store.listBusy && !store.items.length" class="state">正在加载策略市场</p>
    <template v-else-if="store.listError">
      <p class="state err">{{ store.listError }}</p>
      <button type="button" class="zg-btn zg-btn-ghost" @click="store.fetchList()">重试</button>
    </template>
    <template v-else-if="!store.items.length">
      <div class="empty">
        <p>市场暂时没有符合条件的策略。</p>
        <p v-if="hasFilter" class="hint">当前筛选没有结果，可以清除筛选再看看。</p>
        <button v-if="hasFilter" type="button" class="zg-btn zg-btn-ghost" @click="store.resetFilters()">清除筛选</button>
        <RouterLink class="zg-btn zg-btn-ghost" to="/app/strategies">使用 AI 创建策略</RouterLink>
      </div>
    </template>
    <ul v-else class="list">
      <li v-for="item in store.items" :key="item.id">
        <StrategyMarketCard :item="item" />
      </li>
    </ul>
    <button
      v-if="store.nextCursor"
      type="button"
      class="zg-btn zg-btn-ghost more"
      :disabled="store.listBusy"
      @click="store.fetchList({ append: true })"
    >{{ store.listBusy ? '正在加载' : '加载更多' }}</button>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import StrategySubnav from '../../components/strategies/StrategySubnav.vue'
import StrategyMarketCard from '../../components/strategies/StrategyMarketCard.vue'
import { useStrategyMarket } from '../../stores/strategyMarket.js'

const route = useRoute()
const router = useRouter()
const store = useStrategyMarket()

const hasFilter = computed(() => Object.values(store.filters).some((v) => v))
const categoryOptions = computed(() => [...new Set(store.items.map((i) => i.category).filter(Boolean))])
const periodOptions = computed(() => [...new Set(store.items.map((i) => i.signal_period).filter(Boolean))])

function onSearchInput() {
  store.scheduleSearch()
  router.replace({ query: store.queryOf() })
}
function apply(key, event) {
  store.setFilter(key, event.target.value)
  router.replace({ query: store.queryOf() })
}

onMounted(() => {
  store.ensureAccount()
  store.syncQuery(route.query)
  store.fetchList()
  window.scrollTo(0, store.savedScroll)
})
onBeforeUnmount(() => {
  store.saveScroll(window.scrollY)
})
</script>

<style scoped>
.market {
  padding: 12px 20px 40px;
  max-width: 960px;
  margin: 0 auto;
}
.head h2 {
  margin: 12px 0 4px;
}
.hint {
  font-size: 13px;
  color: var(--zg-muted, #6b7280);
}
.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 12px 0;
}
.search {
  flex: 1 1 220px;
  padding: 6px 10px;
  border: 1px solid var(--zg-line, #e5e7eb);
  border-radius: 8px;
}
select {
  padding: 6px 8px;
  border: 1px solid var(--zg-line, #e5e7eb);
  border-radius: 8px;
}
.list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 12px;
}
.state {
  font-size: 14px;
  color: var(--zg-muted, #6b7280);
}
.err {
  color: #b91c1c;
}
.empty {
  border: 1px dashed var(--zg-line, #cbd5e1);
  border-radius: 12px;
  padding: 32px;
  text-align: center;
  display: grid;
  gap: 10px;
  justify-items: center;
}
.more {
  margin-top: 12px;
}
</style>
