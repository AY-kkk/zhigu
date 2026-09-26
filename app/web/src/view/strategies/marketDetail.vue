<template>
  <div class="detail">
    <StrategySubnav />
    <p class="back">
      <RouterLink class="zg-btn zg-btn-ghost" :to="{ path: '/app/strategies/market', query: backQuery }">返回市场列表</RouterLink>
    </p>

    <p v-if="store.detailBusy" class="state">正在加载策略详情</p>
    <template v-else-if="store.detailError">
      <p class="state err">{{ store.detailError }}</p>
      <RouterLink class="zg-btn zg-btn-ghost" :to="{ path: '/app/strategies/market', query: backQuery }">返回市场列表</RouterLink>
    </template>
    <template v-else-if="detail">
      <header class="head">
        <h2>{{ detail.name }} <span class="ver">v{{ detail.version_no }}</span></h2>
        <p class="summary">{{ detail.summary }}</p>
        <p class="chips">
          <span class="badge" :class="detail.validation_status">规则校验：{{ validationText }}</span>
          <span class="badge" :class="detail.backtest_status">回测证据：{{ backtestText }}</span>
          <span class="chip">更新于 {{ dateText(detail.updated_at) }}</span>
        </p>
      </header>

      <section class="block">
        <h3>思路与假设</h3>
        <p>{{ detail.description }}</p>
        <h4>试图捕捉什么</h4>
        <p>{{ detail.hypothesis }}</p>
        <h4>可能失效的情形</h4>
        <p>{{ detail.failure_cases }}</p>
      </section>

      <RuleEditor
        :state="editorState"
        :readonly="true"
        :field-sources="marketDefaultSources"
      />

      <section class="block">
        <h3>操作</h3>
        <div class="ops">
          <button
            type="button"
            class="zg-btn"
            :disabled="!detail.copyable || store.copyBusy"
            :aria-busy="store.copyBusy"
            @click="onCopy"
          >{{ store.copyBusy ? '正在复制…' : '复制到我的策略' }}</button>
          <button
            type="button"
            class="zg-btn zg-btn-ghost"
            :disabled="!detail.copyable || store.copyBusy"
            @click="onExplain"
          >让 AI 解释策略</button>
          <span v-if="!detail.copyable" class="reason">{{ detail.copy_disabled_reason }}</span>
        </div>
        <p v-if="store.copyError" class="err">{{ store.copyError }}</p>
        <div v-if="leavePrompt" class="leave" role="dialog" aria-label="未保存修改">
          <p>工作台有未保存的规则修改。请先保存、放弃修改，或取消本次导航。</p>
          <div class="ops">
            <button type="button" class="zg-btn" @click="leaveSave">先保存并继续</button>
            <button type="button" class="zg-btn zg-btn-ghost" @click="leaveDiscard">放弃修改并继续</button>
            <button type="button" class="zg-btn zg-btn-ghost" @click="leaveCancel">取消导航（不复制）</button>
          </div>
        </div>
      </section>

      <section class="block">
        <h3>历史回测证据</h3>
        <p class="hint">规则校验与回测证据分开展示；未回测时不填任何收益数字。</p>
        <ul class="evi-list">
          <li v-for="sum in detail.evidence_summaries || []" :key="sum.evidence_id">
            <button type="button" class="zg-btn zg-btn-ghost" @click="openEvidence(sum.evidence_id)">
              {{ sum.validation_instrument_id }} · {{ dateText(sum.created_at) }}
            </button>
          </li>
          <li v-if="!(detail.evidence_summaries || []).length" class="hint">未回测（没有审核通过的历史模拟记录）。</li>
        </ul>
        <StrategyEvidencePanel
          v-if="activeEvidenceId"
          :item-id="detail.id"
          :evidence-id="activeEvidenceId"
        />
      </section>

      <StrategyProvenance :sources="detail.sources" :rights-note="detail.rights_note" />
    </template>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import StrategySubnav from '../../components/strategies/StrategySubnav.vue'
import RuleEditor from '../../components/strategies/RuleEditor.vue'
import StrategyProvenance from '../../components/strategies/StrategyProvenance.vue'
import StrategyEvidencePanel from '../../components/strategies/StrategyEvidencePanel.vue'
import { useStrategyMarket } from '../../stores/strategyMarket.js'
import { useStrategyWorkspace } from '../../stores/strategyWorkspace.js'

const route = useRoute()
const router = useRouter()
const store = useStrategyMarket()
const workspace = useStrategyWorkspace()

const activeEvidenceId = ref('')
const leavePrompt = ref(false)
const pendingNav = ref(null)

const detail = computed(() => store.detail)
const backQuery = computed(() => store.queryOf())
const editorState = computed(() => detail.value?.editor_state || {})
const marketDefaultSources = computed(() => {
  const out = {}
  ;['name', 'signal_period', 'price_basis', 'indicators', 'entry', 'exit', 'position', 'risk', 'execution'].forEach((f) => {
    out[f] = 'market_default'
  })
  return out
})
const validationText = computed(
  () => ({ passed: '已通过', failed: '未通过', pending: '未校验' }[detail.value?.validation_status] || '未校验')
)
const backtestText = computed(() => (detail.value?.backtest_status === 'has_evidence' ? '有历史模拟记录' : '未回测'))

function dateText(v) {
  return typeof v === 'string' ? v.slice(0, 10) : ''
}
function openEvidence(evidenceId) {
  activeEvidenceId.value = evidenceId
}

function buildCopyPayload() {
  return {
    market_version_id: detail.value.market_version_id,
    overrides: workspaceCopyOverrides()
  }
}
function workspaceCopyOverrides() {
  // 用户此前在工作台选定的标的与回测配置随复制覆盖；未选不代入验证标的。
  const ov = {}
  if (workspace.instrument?.instrument_id) ov.instrument_id = workspace.instrument.instrument_id
  if (workspace.initialCash) ov.initial_cash = workspace.initialCash
  if (workspace.instrument?.currency) ov.currency = workspace.instrument.currency
  if (workspace.start) ov.start = workspace.start
  if (workspace.end) ov.end = workspace.end
  return Object.keys(ov).length ? ov : undefined
}

async function doCopy() {
  const result = await store.copy(detail.value.id, buildCopyPayload())
  if (result?.draft_id) {
    router.push({ path: '/app/strategies', query: { draft_id: result.draft_id } })
  }
}

function onCopy() {
  if (workspace.hasUnsavedEdits) {
    pendingNav.value = 'copy'
    leavePrompt.value = true
    return
  }
  doCopy()
}
function onExplain() {
  // 解释进入工作台草稿上下文，以 explain 模式生成说明（不改规则）。
  // R5：解释动作必须走 explain 分支；漏设 pendingNav 会误执行复制。
  pendingNav.value = 'explain'
  if (workspace.hasUnsavedEdits) {
    leavePrompt.value = true
    return
  }
  runPending()
}
function leaveSave() {
  workspace.saveCurrent().then(runPending)
}
function leaveDiscard() {
  workspace.discardEdits()
  runPending()
}
function leaveCancel() {
  leavePrompt.value = false
  pendingNav.value = null
}
async function runPending() {
  const kind = pendingNav.value
  leavePrompt.value = false
  pendingNav.value = null
  if (kind === 'explain') {
    // 明确目标草稿：工作台按市场版本复制/加载完成后才执行 explain（R5-H2）。
    router.push({
      path: '/app/strategies',
      query: {
        explain_market: detail.value.id,
        explain_version: detail.value.market_version_id
      }
    })
    return
  }
  await doCopy()
}

onMounted(() => {
  store.ensureAccount()
  store.fetchDetail(route.params.id)
})
</script>

<style scoped>
.detail {
  padding: 12px 20px 40px;
  max-width: 860px;
  margin: 0 auto;
}
.back {
  margin: 10px 0;
}
.head h2 {
  margin: 8px 0 4px;
}
.ver {
  font-size: 13px;
  color: var(--zg-muted, #6b7280);
  font-weight: 400;
}
.summary {
  margin: 4px 0;
  font-size: 14px;
}
.chips {
  margin: 6px 0;
  display: flex;
  gap: 8px;
  align-items: center;
}
.badge {
  padding: 2px 8px;
  border-radius: 6px;
  font-size: 12px;
  background: #fef9c3;
}
.badge.passed,
.badge.has_evidence {
  background: #dcfce7;
}
.badge.failed {
  background: #fee2e2;
}
.chip {
  font-size: 12px;
  color: var(--zg-muted, #6b7280);
}
.block {
  border: 1px solid var(--zg-line, #e5e7eb);
  border-radius: 10px;
  padding: 12px 16px;
  margin: 12px 0;
}
.block h3 {
  margin: 0 0 8px;
  font-size: 15px;
}
.block h4 {
  margin: 10px 0 4px;
  font-size: 13px;
}
.block p {
  margin: 4px 0;
  font-size: 13px;
  line-height: 1.6;
}
.ops {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
}
.reason {
  color: #b45309;
  font-size: 13px;
}
.err {
  color: #b91c1c;
  font-size: 13px;
}
.hint {
  color: var(--zg-muted, #6b7280);
  font-size: 13px;
}
.evi-list {
  list-style: none;
  padding: 0;
  margin: 8px 0;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.leave {
  margin-top: 10px;
  border: 1px solid #f59e0b;
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 13px;
}
.state {
  font-size: 14px;
  color: var(--zg-muted, #6b7280);
}
</style>
