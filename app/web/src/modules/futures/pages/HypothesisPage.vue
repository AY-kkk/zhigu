<template>
  <section class="hypothesis-page">
    <PageHeader
      title="研究假设"
      :summary="hypothesis?.proposition || '从服务端恢复假设生命周期、校验条件和复盘。'"
      :status="hypothesis ? `${hypothesis.lifecycle} · v${hypothesis.version}` : '读取中'"
      :secondary-actions="availableActions"
      @secondary="prepareLifecycle"
    />

    <RequestError v-if="error" :error="error" @retry="load" />
    <RequestError v-if="actionError" :error="actionError" />
    <LoadingState v-else-if="!hypothesis" label="正在读取假设" :rows="5" />

    <template v-else>
      <section class="hypothesis-summary">
        <dl>
          <dt>方向/判断</dt><dd>{{ hypothesis.user_view }}</dd>
          <dt>来源 run</dt><dd><RouterLink :to="`/app/futures/research/${encodeURIComponent(hypothesis.run_id)}`">{{ hypothesis.run_id }}</RouterLink></dd>
          <dt>观察期</dt><dd>创建 {{ hypothesis.created_at }} · 失效 {{ hypothesis.expires_at }}</dd>
          <dt>前提</dt><dd>{{ assumptions || '未提供结构化前提' }}</dd>
          <dt>反证</dt><dd>{{ counterevidence || '未提供结构化反证' }}</dd>
        </dl>
      </section>

      <section>
        <h2>校验条件</h2>
        <div class="condition-list">
          <article v-for="condition in hypothesis.conditions || []" :key="condition.id">
            <strong>{{ condition.metric || condition.instruction || condition.role }}</strong>
            <p>{{ readableCondition(condition) }}</p>
            <p>来源：{{ condition.source || '未提供' }}</p>
            <p v-if="condition.status === 'waiting' || condition.status === 'blocked'" class="waiting">
              {{ condition.status }}：数据不足，不能解释为条件未触发。
            </p>
          </article>
        </div>
      </section>

      <section>
        <h2>最新条件检查</h2>
        <EmptyState
          v-if="!latestCheck"
          title="暂无可复核检查"
          description="首次决策需等待服务端完成条件检查；无法计算不等于条件未触发。"
        />
        <template v-else>
          <p>检查版本 {{ latestCheck.version }} · {{ latestCheck.result }} · {{ latestCheck.checked_at }}</p>
          <p>{{ latestCheck.reason }}</p>
          <button
            v-if="hypothesis.reviewed_check_version !== latestCheck.version"
            type="button"
            @click="acknowledge"
          >
            确认已复核
          </button>
        </template>
      </section>

      <section>
        <h2>用户决策</h2>
        <form class="decision-form" @submit.prevent="prepareDecision">
          <label>判断
            <select v-model="view">
              <option value="retained">保留</option>
              <option value="revised">修改</option>
              <option value="rejected">否定</option>
            </select>
          </label>
          <label>理由<input v-model.trim="reason" required></label>
          <button type="submit" :disabled="!canDecide">记录决策</button>
        </form>
      </section>

      <section>
        <h2>四维复盘</h2>
        <form class="recap-form" @submit.prevent="saveRecap">
          <label>事实兑现<textarea v-model.trim="recap.facts" required></textarea></label>
          <label>传导支持<textarea v-model.trim="recap.transmission" required></textarea></label>
          <label>实际合约表现<textarea v-model.trim="recap.contract_performance" required></textarea></label>
          <label>数据充分性<textarea v-model.trim="recap.data_sufficiency" required></textarea></label>
          <label>复盘理由<textarea v-model.trim="recap.reason" required></textarea></label>
          <button type="submit">保存复盘</button>
        </form>
      </section>
    </template>

    <ConfirmAction
      :open="Boolean(pendingAction)"
      :title="pendingAction?.label || '确认状态变更'"
      :impact="pendingAction?.impact || ''"
      :busy="busy"
      @cancel="pendingAction = null"
      @confirm="executePendingAction"
    />
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import ConfirmAction from '../../../components/system/ConfirmAction.vue'
import EmptyState from '../../../components/system/EmptyState.vue'
import LoadingState from '../../../components/system/LoadingState.vue'
import RequestError from '../../../components/system/RequestError.vue'
import PageHeader from '../../../components/workspace/PageHeader.vue'
import { getChecks, getHypothesis, getRecap, patchHypothesis, putRecap } from '../api.js'

const route = useRoute()
const hypothesis = ref(null)
const checks = ref([])
const recapVersion = ref(1)
const error = ref(null)
const actionError = ref(null)
const view = ref('retained')
const reason = ref('')
const busy = ref(false)
const pendingAction = ref(null)
const recap = ref({ facts: '', transmission: '', contract_performance: '', data_sufficiency: '', reason: '' })
const latestCheck = computed(() => checks.value[0] || null)
const canDecide = computed(() => Boolean(latestCheck.value && hypothesis.value?.reviewed_check_version === latestCheck.value.version && reason.value.trim()))
const assumptions = computed(() => (hypothesis.value?.assumptions || []).join('；'))
const counterevidence = computed(() => (hypothesis.value?.counterevidence || []).join('；'))

const availableActions = computed(() => {
  const lifecycle = hypothesis.value?.lifecycle
  const actions = []
  if (lifecycle === 'draft') actions.push({ action: 'activate', label: '激活', impact: '假设进入 active，可按条件检查持续验证。' })
  if (lifecycle === 'active') actions.push({ action: 'pause', label: '暂停', impact: '停止主动验证，但保留假设和历史。' })
  if (lifecycle === 'paused') actions.push({ action: 'resume', label: '恢复', impact: '重新进入 active 并继续条件检查。' })
  if (!['closed', 'deleted'].includes(lifecycle)) actions.push({ action: 'close', label: '关闭', impact: '关闭假设；不会删除来源 run、证据或复盘记录。' })
  return actions
})

function readableCondition(condition) {
  if (condition.operator && condition.threshold !== undefined) {
    return `${condition.metric} ${condition.operator} ${condition.threshold} · 窗口 ${condition.window || '未提供'}`
  }
  return condition.instruction || '条件表达式未提供'
}

function prepareLifecycle(action) {
  pendingAction.value = action
}

function prepareDecision() {
  pendingAction.value = {
    action: 'decide',
    label: '记录用户决策',
    impact: `将判断更新为 ${view.value}，并记录理由和已复核检查版本。`,
  }
}

async function load() {
  error.value = null
  try {
    const [value, checkPage, currentRecap] = await Promise.all([
      getHypothesis(route.params.id),
      getChecks(route.params.id, undefined, 100).catch(() => ({ items: [] })),
      getRecap(route.params.id).catch(() => null),
    ])
    hypothesis.value = value
    checks.value = checkPage.items || []
    if (currentRecap) {
      recapVersion.value = currentRecap.version
      recap.value = {
        facts: currentRecap.facts,
        transmission: currentRecap.transmission,
        contract_performance: currentRecap.contract_performance,
        data_sufficiency: currentRecap.data_sufficiency,
        reason: currentRecap.reason,
      }
    }
  } catch (failure) {
    error.value = failure
  }
}

async function acknowledge() {
  actionError.value = null
  busy.value = true
  try {
    hypothesis.value = await patchHypothesis(route.params.id, {
      expected_version: hypothesis.value.version,
      action: 'acknowledge',
      reviewed_check_version: latestCheck.value.version,
    })
  } catch (failure) {
    actionError.value = failure
  } finally {
    busy.value = false
  }
}

async function executePendingAction() {
  const item = pendingAction.value
  if (!item) return
  busy.value = true
  actionError.value = null
  try {
    const body = item.action === 'decide'
      ? {
          expected_version: hypothesis.value.version,
          action: 'decide',
          user_view: view.value,
          reason: reason.value,
          reviewed_check_version: latestCheck.value.version,
        }
      : { expected_version: hypothesis.value.version, action: item.action }
    hypothesis.value = await patchHypothesis(route.params.id, body)
    pendingAction.value = null
  } catch (failure) {
    actionError.value = failure
  } finally {
    busy.value = false
  }
}

async function saveRecap() {
  actionError.value = null
  busy.value = true
  try {
    const saved = await putRecap(route.params.id, {
      hypothesis_id: route.params.id,
      version: recapVersion.value,
      ...recap.value,
    })
    recapVersion.value = saved.version
    await load()
  } catch (failure) {
    actionError.value = failure
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.hypothesis-summary, .condition-list article, .hypothesis-page > section { padding: 18px; border: 1px solid var(--zg-line, #dedbd4); border-radius: 12px; margin-bottom: 16px; }
.hypothesis-summary dl { display: grid; grid-template-columns: max-content 1fr; gap: 10px 18px; }
.condition-list { display: grid; gap: 10px; }
.waiting { color: var(--zg-warning, #8a5600); font-weight: 650; }
.decision-form, .recap-form { display: grid; gap: 12px; max-width: 760px; }
label { display: grid; gap: 5px; font-weight: 650; }
input, select, textarea { width: 100%; padding: 10px; border: 1px solid var(--zg-line, #dedbd4); border-radius: 8px; font: inherit; }
textarea { min-height: 92px; }
button { justify-self: start; min-height: 40px; padding: 8px 13px; border: 1px solid currentColor; border-radius: 8px; }
</style>
