<template>
  <section class="evi" aria-label="历史回测证据">
    <header class="head">
      <h3>历史回测证据</h3>
      <p class="note">以下为平台审核通过的历史模拟记录，与你的私人回测结果分开展示，不代表未来收益。</p>
    </header>
    <div class="tabs" role="tablist">
      <button
        v-for="s in sections"
        :key="s.key"
        type="button"
        role="tab"
        class="tab"
        :class="{ on: store.evidenceSection === s.key }"
        :aria-selected="store.evidenceSection === s.key"
        @click="switchSection(s.key)"
      >{{ s.label }}</button>
    </div>
    <p v-if="store.evidenceBusy" class="state">正在加载证据</p>
    <p v-else-if="store.evidenceError" class="state err">{{ store.evidenceError }}</p>
    <template v-else>
      <div v-if="store.evidenceSection === 'overview'" class="overview">
        <dl v-for="(row, i) in store.evidenceItems" :key="i">
          <div><dt>验证标的</dt><dd>{{ row.validation_instrument_id }}</dd></div>
          <div><dt>结果 hash</dt><dd class="mono">{{ row.result_hash }}</dd></div>
          <div><dt>指标</dt><dd class="mono">{{ jsonText(row.metrics) }}</dd></div>
          <div><dt>配置</dt><dd class="mono">{{ jsonText(row.config) }}</dd></div>
          <div><dt>运行说明</dt><dd class="mono">{{ jsonText(row.manifest) }}</dd></div>
          <div><dt>限制</dt><dd>{{ listText(row.limitations) }}</dd></div>
        </dl>
      </div>
      <ul v-else-if="store.evidenceSection === 'equity'" class="rows">
        <li v-for="(row, i) in store.evidenceItems" :key="i" class="mono">
          {{ row.date }} 权益 {{ row.equity }} 现金 {{ row.cash }} 持仓 {{ row.position_qty }}
        </li>
      </ul>
      <ul v-else class="rows">
        <li v-for="(row, i) in store.evidenceItems" :key="i" class="mono">
          {{ row.date }} {{ row.side }} {{ row.status }} 数量 {{ row.qty }} 价格 {{ row.price }} 费用 {{ jsonText(row.fees) }}
        </li>
      </ul>
      <p v-if="!store.evidenceItems.length" class="state">该部分暂无记录。</p>
      <button
        v-if="store.evidenceNext"
        type="button"
        class="zg-btn zg-btn-ghost"
        :disabled="store.evidenceBusy"
        @click="store.loadEvidence(evidenceId, store.evidenceSection, { append: true })"
      >加载更多</button>
    </template>
  </section>
</template>

<script setup>
import { onMounted } from 'vue'
import { useStrategyMarket } from '../../stores/strategyMarket.js'

const props = defineProps({
  itemId: { type: String, required: true },
  evidenceId: { type: String, required: true }
})
const store = useStrategyMarket()
const sections = [
  { key: 'overview', label: '概览' },
  { key: 'equity', label: '净值曲线' },
  { key: 'trades', label: '成交明细' }
]

function switchSection(key) {
  store.loadEvidence(props.evidenceId, key)
}
function jsonText(v) {
  return typeof v === 'string' ? v : JSON.stringify(v ?? {})
}
function listText(v) {
  if (Array.isArray(v)) return v.join('；') || '无'
  return jsonText(v)
}
onMounted(() => store.loadEvidence(props.evidenceId, 'overview'))
</script>

<style scoped>
.evi {
  border: 1px solid var(--zg-line, #e5e7eb);
  border-radius: 10px;
  padding: 12px 16px;
}
.head h3 {
  margin: 0;
  font-size: 15px;
}
.note {
  margin: 4px 0 8px;
  font-size: 12px;
  color: var(--zg-muted, #6b7280);
}
.tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}
.tab {
  border: 1px solid var(--zg-line, #e5e7eb);
  background: transparent;
  border-radius: 6px;
  padding: 4px 10px;
  font-size: 13px;
  cursor: pointer;
}
.tab.on {
  border-color: var(--zg-brand, #1d4ed8);
  color: var(--zg-brand, #1d4ed8);
}
.state {
  font-size: 13px;
  color: var(--zg-muted, #6b7280);
}
.err {
  color: #b91c1c;
}
dl div {
  display: flex;
  gap: 8px;
  font-size: 13px;
  margin-bottom: 4px;
}
dt {
  width: 72px;
  color: var(--zg-muted, #6b7280);
  flex: none;
}
dd {
  margin: 0;
  word-break: break-all;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
}
.rows {
  margin: 0;
  padding-left: 18px;
  font-size: 12px;
}
.rows li {
  margin-bottom: 3px;
}
</style>
