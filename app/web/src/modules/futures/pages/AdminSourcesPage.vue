<template>
  <section class="admin-sources">
    <PageHeader
      title="期货来源管理"
      summary="来源权限、适配器、探针和发布闸门必须结构化；secret 只写不回显。"
    />

    <RequestError v-if="error" :error="error" @retry="load" />

    <section class="admin-section">
      <h2>运行模式与发布闸门</h2>
      <div class="form-grid">
        <label>目标模式
          <select v-model="operation.mode">
            <option value="off">off</option>
            <option value="read_only">read_only</option>
            <option value="live">live</option>
          </select>
        </label>
        <label>当前版本<input v-model.number="operation.expected_version" type="number" min="1"></label>
        <label>变更原因<input v-model.trim="operation.reason" placeholder="显示给审计和用户的状态原因"></label>
        <label v-if="operation.mode === 'live'">Release gate ID<input v-model.trim="operation.release_gate_id"></label>
        <label v-if="operation.mode === 'live'">Config digest<input v-model.trim="operation.config_digest" pattern="[0-9a-f]{64}"></label>
      </div>
      <button type="button" class="primary" :disabled="!operationValid || busy" @click="confirmLive = true">
        提交运行变更
      </button>
    </section>

    <section class="admin-section">
      <h2>来源清单</h2>
      <div class="form-grid">
        <label>来源 ID<input v-model.trim="manifest.source_id"></label>
        <label>版本<input v-model.number="manifest.version" type="number" min="1"></label>
        <label>发布者<input v-model.trim="manifest.publisher"></label>
        <label>URL<input v-model.trim="manifest.url" type="url"></label>
        <label>Locator 方法<input v-model.trim="manifest.locator_method"></label>
        <label>Secret 引用（只写）<input v-model.trim="manifest.credential_ref" type="password" autocomplete="new-password"></label>
        <label>适配器版本<input v-model.trim="manifest.adapter_version"></label>
        <label>责任人<input v-model.trim="manifest.responsible_role"></label>
        <label>覆盖开始<input v-model="manifest.coverage_start" type="date"></label>
        <label>覆盖结束<input v-model="manifest.coverage_end" type="date"></label>
        <label>指标（逗号分隔）<input v-model="metricsText"></label>
        <label>调度频率
          <select v-model="manifest.schedule.frequency">
            <option>daily</option><option>weekly</option><option>monthly</option><option>irregular</option>
          </select>
        </label>
      </div>
      <button type="button" :disabled="!manifestValid || busy" @click="submitSource">登记来源</button>
    </section>

    <section class="admin-section">
      <h2>来源探针</h2>
      <div class="form-grid">
        <label>来源 ID<select v-model="probe.source_id"><option disabled value="">请选择</option><option v-for="source in sources" :key="source.id" :value="source.id">{{ source.publisher }} · {{ source.id }}</option></select></label>
        <label>来源版本<input v-model.number="probe.source_version" type="number" min="1"></label>
        <label>适配器版本<input v-model.trim="probe.adapter_version"></label>
        <label>结果
          <select v-model="probe.result"><option>PASS</option><option>FAIL</option><option>NOT_RUN</option><option>BLOCKED</option></select>
        </label>
        <label>原因<input v-model.trim="probe.reason"></label>
        <label>Secret revision<input v-model.number="probe.secret_revision" type="number" min="1"></label>
        <label>Secret fingerprint<input v-model.trim="probe.secret_fingerprint" type="password" autocomplete="off"></label>
        <label>Evidence hash<input v-model.trim="probe.evidence_hash" pattern="[0-9a-f]{64}"></label>
      </div>
      <button type="button" :disabled="!probeValid || busy" @click="submitProbe">执行并记录探针</button>
      <div v-if="sourceProbes.length" class="probe-results">
        <article v-for="item in sourceProbes" :key="item.id">
          <strong>{{ item.result }}</strong>
          <p>{{ item.sourceId }} · {{ item.adapterVersion }}</p>
          <p>config_digest：{{ item.configDigest }}</p>
          <p>evidence_hash：{{ item.evidenceHash }}</p>
          <p v-if="item.releaseGateId">release_gate_id：{{ item.releaseGateId }}</p>
        </article>
      </div>
    </section>

    <section class="admin-section">
      <h2>来源状态</h2>
      <div class="table-scroll" tabindex="0">
        <table>
          <thead><tr><th>ID</th><th>版本</th><th>发布者</th><th>状态</th><th>健康</th><th>权限</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="source in sources" :key="source.id">
              <td>{{ source.id }}</td><td>{{ source.version }}</td><td>{{ source.publisher }}</td>
              <td>{{ source.status }}</td><td>{{ source.health }}</td>
              <td>{{ source.rights?.display ? '可展示' : '不可展示' }}</td>
              <td><button type="button" @click="prepareRevoke(source)">撤销</button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <ConfirmAction
      :open="confirmLive"
      title="切换期货运行模式"
      :impact="operation.mode === 'live' ? '将消费指定 release gate 并允许正式付费研究。配置摘要、版本或闸门不匹配时服务端必须拒绝。' : '将切换到非 live 模式并停止新的付费研究。'"
      :busy="busy"
      @cancel="confirmLive = false"
      @confirm="saveOperations"
    />
    <ConfirmAction
      :open="Boolean(revokeTarget)"
      title="撤销来源"
      :impact="`将来源 ${revokeTarget?.id || ''} 标记为 revoked。已有数据保留审计记录，但不能用于新的研究。`"
      confirm-label="确认撤销"
      :busy="busy"
      @cancel="revokeTarget = null"
      @confirm="revokeSource"
    />
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import ConfirmAction from '../../../components/system/ConfirmAction.vue'
import RequestError from '../../../components/system/RequestError.vue'
import PageHeader from '../../../components/workspace/PageHeader.vue'
import {
  getOperations,
  getSources,
  patchSource,
} from '../api.js'
import { normalizeSourceProbe } from '../model.js'
import { useFuturesStore } from '../store.js'

const store = useFuturesStore()
const operation = reactive({
  expected_version: 1,
  mode: 'off',
  reason: '',
  allowed_user_ids: [],
  release_gate_id: '',
  config_digest: '',
})
const manifest = reactive({
  source_id: '',
  version: 1,
  publisher: '',
  url: '',
  locator_method: 'https-json',
  credential_ref: '',
  rights: { display: true, model_use: true, export: false, retain_until: new Date(Date.now() + 365 * 86400000).toISOString(), version: 'rights-v1' },
  metrics: [],
  field_mapping: [],
  schedule: { frequency: 'daily', timezone: 'Asia/Shanghai', calendar_version: 'calendar-v1', rule: 'daily', precision: 'date' },
  pagination: { method: 'cursor', termination_rule: 'empty-page' },
  coverage_start: '',
  coverage_end: '',
  requests_per_minute: 30,
  timeout_seconds: 20,
  max_retries: 2,
  retry_interval_seconds: 900,
  priority: 1,
  adapter_version: '',
  responsible_role: '',
})
const probe = reactive({
  source_id: '',
  source_version: 1,
  adapter_version: '',
  result: 'NOT_RUN',
  reason: '',
  secret_revision: 1,
  secret_fingerprint: '',
  evidence_hash: '',
})
const metricsText = ref('')
const sources = ref([])
const sourceProbes = ref([])
const error = ref(null)
const busy = ref(false)
const confirmLive = ref(false)
const revokeTarget = ref(null)

const operationValid = computed(() => operation.expected_version >= 1 && operation.reason && (operation.mode !== 'live' || (operation.release_gate_id && /^[0-9a-f]{64}$/.test(operation.config_digest))))
const manifestValid = computed(() => manifest.source_id && manifest.publisher && /^https:\/\//.test(manifest.url) && manifest.adapter_version && manifest.responsible_role && manifest.coverage_start && manifest.coverage_end)
const probeValid = computed(() => probe.source_id && probe.source_version >= 1 && probe.adapter_version && probe.reason && probe.secret_revision >= 1 && /^[A-Za-z0-9_-]{32,128}$/.test(probe.secret_fingerprint) && /^[0-9a-f]{64}$/.test(probe.evidence_hash))

async function load() {
  error.value = null
  try {
    const [sourcePage, current] = await Promise.all([getSources(undefined, 100), getOperations()])
    sources.value = (sourcePage.items || []).map(item => ({ ...item }))
    Object.assign(operation, current, { release_gate_id: '', config_digest: '' })
  } catch (reason) {
    error.value = reason
  }
}

async function saveOperations() {
  busy.value = true
  try {
    const body = {
      expected_version: operation.expected_version,
      mode: operation.mode,
      reason: operation.reason,
      allowed_user_ids: operation.allowed_user_ids,
    }
    if (operation.mode === 'live') {
      body.release_gate_id = operation.release_gate_id
      body.config_digest = operation.config_digest
    }
    await store.activateLive(body)
    confirmLive.value = false
    await load()
  } catch (reason) {
    error.value = reason
  } finally {
    busy.value = false
  }
}

async function submitSource() {
  busy.value = true
  try {
    const body = { ...manifest, metrics: metricsText.value.split(',').map(item => item.trim()).filter(Boolean) }
    const api = await import('../api.js')
    await api.createSource(body, crypto.randomUUID())
    await load()
  } catch (reason) {
    error.value = reason
  } finally {
    busy.value = false
  }
}

async function submitProbe() {
  busy.value = true
  try {
    const value = await store.runSourceProbe({ ...probe })
    sourceProbes.value = [normalizeSourceProbe(value), ...sourceProbes.value]
  } catch (reason) {
    error.value = reason
  } finally {
    busy.value = false
  }
}

function prepareRevoke(source) {
  revokeTarget.value = source
}

async function revokeSource() {
  const source = revokeTarget.value
  if (!source) return
  busy.value = true
  try {
    await patchSource(source.id, {
      expected_version: source.version,
      status: 'revoked',
      reason: '管理员撤销来源；阻止新研究使用',
    })
    revokeTarget.value = null
    await load()
  } catch (reason) {
    error.value = reason
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.admin-section { margin: 22px 0; padding: 20px; border: 1px solid var(--zg-line, #dedbd4); border-radius: 12px; }
.form-grid { display: grid; grid-template-columns: repeat(3, minmax(180px, 1fr)); gap: 12px; margin-bottom: 14px; }
.form-grid label { display: grid; gap: 5px; font-weight: 650; }
.form-grid input, .form-grid select { min-height: 40px; padding: 8px 10px; border: 1px solid var(--zg-line, #dedbd4); border-radius: 8px; }
button { min-height: 40px; padding: 8px 13px; border: 1px solid currentColor; border-radius: 8px; }
button.primary { color: white; background: var(--zg-action, #b4232c); border-color: var(--zg-action, #b4232c); }
.table-scroll { overflow-x: auto; }
table { width: 100%; min-width: 820px; border-collapse: collapse; }
th, td { padding: 11px; text-align: left; border-bottom: 1px solid var(--zg-line, #dedbd4); }
.probe-results { display: grid; gap: 10px; margin-top: 14px; }
.probe-results article { padding: 12px; border: 1px solid var(--zg-line, #dedbd4); }
@media (max-width: 767px) { .form-grid { grid-template-columns: 1fr; } }
</style>
