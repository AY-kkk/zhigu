<template>
  <div v-if="windowMode" class="win-overlay" @click.self="tryClose"></div>
  <section class="rule" :class="{ win: windowMode }" :role="windowMode ? 'dialog' : undefined" :aria-label="windowMode ? '配置买卖规则' : undefined">
    <header class="head">
      <h3>{{ windowMode ? '配置买卖规则' : '买卖规则与具体参数' }}</h3>
      <button v-if="windowMode" type="button" class="close" aria-label="关闭" @click="tryClose">×</button>
      <p v-if="conflict" class="warn" role="alert">{{ conflict }}</p>
      <p v-if="current.name" class="name">{{ current.name }}</p>
      <p class="meta">
        <span>信号周期：{{ current.signal_period }}</span>
        <span>价格口径：{{ current.price_basis === 'raw' ? '原始价格' : '因果前复权' }}</span>
        <span v-if="current.instrument_id">标的：{{ current.instrument_id }}</span>
        <span v-else class="warn">标的待补充</span>
        <button
          v-if="editable && !current.instrument_id && fallbackInstrument"
          type="button"
          class="zg-btn zg-btn-ghost"
          data-testid="use-workspace-instrument"
          @click="patchSection('instrument_id', fallbackInstrument)"
        >使用当前行情标的（{{ fallbackInstrument }}）</button>
      </p>
      <p class="summary"><span class="dim">规则摘要（本地生成，不是 AI 解释）：</span>{{ plainSummary }}</p>
    </header>

    <div class="ind">
      <h4>指标参数</h4>
      <ul>
        <li v-for="spec in current.indicators || []" :key="spec.id">
          <strong>{{ spec.type }}</strong>（{{ spec.id }}）：{{ paramText(spec.params) }}
          <span v-if="sourceOf('indicators')" class="src">来源：{{ sourceText('indicators') }}</span>
          <div v-if="editable" class="edit-row">
            <span v-for="(val, key) in spec.params" :key="key" class="param">{{ key }}
              <input
                type="number"
                :aria-label="`${spec.type} 参数 ${key}`"
                :value="val"
                @change="patchIndicatorParam(spec.id, key, $event.target.value)"
              >
            </span>
            <button type="button" class="zg-btn zg-btn-ghost" :data-testid="`remove-indicator-${spec.id}`" @click="removeIndicator(spec.id)">删除指标</button>
          </div>
        </li>
        <li v-if="!(current.indicators || []).length" class="dim">仅使用价格／成交量字段，无指标实例。</li>
      </ul>
      <label v-if="editable" class="edit-row">添加指标
        <select data-testid="add-indicator" aria-label="添加指标" @change="addIndicator($event)">
          <option value="">选择类型</option>
          <option v-for="t in indicatorTypes" :key="t" :value="t">{{ t }}</option>
        </select>
      </label>
    </div>

    <div class="cond">
      <h4>买入条件 <span class="src">{{ sourceText('entry') }}</span></h4>
      <RuleGroup v-if="isGroup(current.entry)" :node="current.entry" :readonly="!editable" @update:node="patchSection('entry', $event)" />
      <RuleConditionRow v-else-if="current.entry" :node="current.entry" :readonly="!editable" @update:node="patchSection('entry', $event)" @remove="patchSection('entry', null)" />
      <h4>卖出条件 <span class="src">{{ sourceText('exit') }}</span></h4>
      <RuleGroup v-if="isGroup(current.exit)" :node="current.exit" :readonly="!editable" @update:node="patchSection('exit', $event)" />
      <RuleConditionRow v-else-if="current.exit" :node="current.exit" :readonly="!editable" @update:node="patchSection('exit', $event)" @remove="patchSection('exit', null)" />
    </div>

    <div class="risk">
      <h4>仓位与风险控制</h4>
      <template v-if="editable">
        <label class="edit-row">仓位（0～1 目标权益比例）
          <input
            data-testid="position-value"
            type="number"
            min="0"
            max="1"
            step="0.01"
            :value="current.position?.value ?? ''"
            @change="patchPositionValue($event.target.value)"
          >
          <span v-if="!current.position?.value" class="warn">待补充</span>
        </label>
        <label class="edit-row">止损（0～1 比例，留空＝明确未启用）
          <input data-testid="stop-loss" type="number" min="0" max="1" step="0.01" :value="current.risk?.stop_loss_pct ?? ''" @change="patchRiskField('stop_loss_pct', $event.target.value)">
        </label>
        <label class="edit-row">止盈（0～1 比例，留空＝明确未启用）
          <input data-testid="take-profit" type="number" min="0" max="1" step="0.01" :value="current.risk?.take_profit_pct ?? ''" @change="patchRiskField('take_profit_pct', $event.target.value)">
        </label>
        <label class="edit-row">最大持有（正整数信号日，留空＝不限）
          <input data-testid="max-holding" type="number" min="1" step="1" :value="current.risk?.max_holding_bars ?? ''" @change="patchRiskField('max_holding_bars', $event.target.value)">
        </label>
        <p v-if="riskHint" class="warn" role="alert" data-testid="risk-hint">{{ riskHint }}</p>
      </template>
      <p>
        仓位：目标权益 {{ current.position?.value || '待补充' }}（{{ sourceText('position') }}）；
        止损：{{ pctText(current.risk?.stop_loss_pct) }}；
        止盈：{{ pctText(current.risk?.take_profit_pct) }}；
        最大持有：{{ current.risk?.max_holding_bars ? `${current.risk.max_holding_bars} 个信号日` : '不限' }}
      </p>
      <p class="dim">风险检查：每日收盘检查本轮持仓净收益率；执行：下一交易日开盘模拟执行、退出优先、退出当日不重新入场。</p>
    </div>

    <footer v-if="windowMode" class="foot">
      <p v-if="confirmLeave" class="warn">窗口内有未应用的修改。</p>
      <div class="ops">
        <template v-if="confirmLeave">
          <button type="button" class="zg-btn zg-btn-ghost" @click="confirmLeave = false">保留并继续编辑</button>
          <button type="button" class="zg-btn zg-btn-ghost" @click="discardAndClose">放弃修改</button>
        </template>
        <template v-else>
          <button type="button" class="zg-btn zg-btn-ghost" @click="tryClose">取消</button>
          <button type="button" class="zg-btn" data-testid="apply-rules" :disabled="!dirty" @click="apply">应用修改</button>
        </template>
      </div>
    </footer>
  </section>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import RuleGroup from './RuleGroup.vue'
import RuleConditionRow from './RuleConditionRow.vue'

const props = defineProps({
  state: { type: Object, required: true },
  readonly: { type: Boolean, default: true },
  windowMode: { type: Boolean, default: false },
  conflict: { type: String, default: '' },
  fieldSources: { type: Object, default: () => ({}) },
  fallbackInstrument: { type: String, default: '' }
})
const emit = defineEmits(['update:state', 'apply', 'cancel'])

// 窗口模式使用编辑副本；应用才回写，取消/关闭不改草稿。
const working = ref(clone(props.state))
const dirty = ref(false)
const confirmLeave = ref(false)

watch(
  () => props.state,
  (v) => {
    working.value = clone(v)
    dirty.value = false
    confirmLeave.value = false
  }
)

const editable = computed(() => !props.readonly || props.windowMode)
const current = computed(() => (props.windowMode ? working.value : props.state))

const plainSummary = computed(() => {
  const entry = condSummary(current.value.entry)
  const exit = condSummary(current.value.exit)
  return `当日收盘 ${entry} 时产生买入信号；${exit} 时退出全部可卖持仓。信号与模拟成交点分开标记，历史模拟不代表真实持仓。`
})

function clone(v) {
  return JSON.parse(JSON.stringify(v || {}))
}
function isGroup(node) {
  return Boolean(node && typeof node === 'object' && (node.all || node.any || node.not))
}
function condSummary(node) {
  if (!node) return '（待补充）'
  if (node.kind === 'volume_increase') return `最近连续 ${node.days} 个交易日成交量逐日增加`
  if (node.all || node.any) {
    const list = node.all || node.any
    return list.map(condSummary).join(node.all ? ' 且 ' : ' 或 ')
  }
  if (node.not) return `非（${condSummary(node.not)}）`
  return `${operandText(node.left)} ${opText(node.op)} ${operandText(node.right)}`
}
function operandText(op) {
  if (typeof op === 'string') return op
  if (op && op.constant != null) return `常数 ${op.constant}`
  if (op && op.ref) return op.lag ? `${op.ref}[滞后 ${op.lag} 日]` : op.ref
  return '（缺失）'
}
function opText(op) {
  return { gt: '高于', gte: '不低于', lt: '低于', lte: '不高于', eq: '等于', crosses_above: '上穿', crosses_below: '下穿' }[op] || op
}
function paramText(params) {
  const entries = Object.entries(params || {})
  if (!entries.length) return '无参数'
  return entries.map(([k, v]) => `${k}=${v}`).join('，')
}
function pctText(v) {
  return v ? `${Number(v) * 100}%` : '未启用'
}
function sourceOf(field) {
  return Boolean(props.fieldSources[field])
}
function sourceText(field) {
  const map = {
    user: '来自你的描述',
    context: '来自当前上下文',
    system_default: '系统默认',
    market_default: '策略原版参数',
    ai_suggestion: 'AI 建议，可修改'
  }
  return map[props.fieldSources[field]] || ''
}
function patchSection(section, next) {
  if (!editable.value) return
  const base = clone(current.value)
  base[section] = next
  if (props.windowMode) {
    working.value = base
    dirty.value = true
    return
  }
  emit('update:state', base)
}

// R6：完整参数配置——指标引用增删、参数、仓位与风险字段，全部走同一编辑副本。
const indicatorTypes = ['MA', 'EMA', 'BOLL', 'MACD', 'KDJ', 'RSI', 'WR', 'BIAS', 'CCI', 'ATR', 'OBV', 'VOL']
const defaultIndicatorParams = {
  MA: { n: 20 },
  EMA: { n: 12 },
  BOLL: { n: 20, k: 2 },
  MACD: { fast: 12, slow: 26, signal: 9 },
  KDJ: { n: 9, m1: 3, m2: 3 },
  RSI: { n: 12 },
  WR: { n: 14 },
  BIAS: { n: 12 },
  CCI: { n: 14 },
  ATR: { n: 14 },
  OBV: {},
  VOL: { ma5: 5, ma10: 10 }
}

const riskHint = computed(() => {
  const r = current.value.risk || {}
  for (const [field, label] of [['stop_loss_pct', '止损'], ['take_profit_pct', '止盈']]) {
    const v = r[field]
    if (v != null && v !== '' && !(Number(v) > 0 && Number(v) <= 1)) {
      return `${label}必须是 0～1 的比例`
    }
  }
  const mhb = r.max_holding_bars
  if (mhb != null && mhb !== '' && !(Number.isInteger(Number(mhb)) && Number(mhb) >= 1)) {
    return '最大持有必须是正整数'
  }
  return ''
})

function patchPositionValue(v) {
  patchSection('position', { ...(current.value.position || {}), type: 'equity_fraction', value: String(v) })
}
function patchRiskField(field, v) {
  const risk = { ...(current.value.risk || {}), check: 'close' }
  if (field === 'max_holding_bars') {
    risk[field] = v === '' ? null : Number(v)
  } else {
    risk[field] = v === '' ? null : String(v)
  }
  patchSection('risk', risk)
}
function patchIndicatorParam(id, key, val) {
  const n = Number(val)
  const list = (current.value.indicators || []).map((s) =>
    s.id === id ? { ...s, params: { ...s.params, [key]: Number.isFinite(n) ? n : s.params[key] } } : s
  )
  patchSection('indicators', list)
}
function removeIndicator(id) {
  patchSection('indicators', (current.value.indicators || []).filter((s) => s.id !== id))
}
function addIndicator(ev) {
  const type = ev.target.value
  ev.target.value = ''
  if (!type) return
  const list = current.value.indicators || []
  if (list.length >= 20) return
  patchSection('indicators', [
    ...list,
    { id: `${type.toLowerCase()}${Date.now()}`, type, params: { ...(defaultIndicatorParams[type] || {}) } }
  ])
}
function apply() {
  emit('apply', clone(working.value))
  dirty.value = false
  confirmLeave.value = false
}
function tryClose() {
  if (dirty.value) {
    confirmLeave.value = true
    return
  }
  emit('cancel')
}
function discardAndClose() {
  working.value = clone(props.state)
  dirty.value = false
  confirmLeave.value = false
  emit('cancel')
}
</script>

<style scoped>
.rule {
  border: 1px solid var(--zg-line, #e5e7eb);
  border-radius: 10px;
  padding: 12px 16px;
  background: var(--zg-panel, #fff);
}
.rule.win {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: 220;
  width: min(560px, 92vw);
  overflow: auto;
  border-radius: 0;
  box-shadow: -8px 0 24px rgba(0, 0, 0, 0.18);
}
@media (max-width: 767px) {
  .rule.win {
    width: 100vw;
  }
}
.win-overlay {
  position: fixed;
  inset: 0;
  z-index: 210;
  background: rgba(0, 0, 0, 0.35);
}
.head h3 {
  margin: 0;
  font-size: 15px;
}
.close {
  position: absolute;
  top: 10px;
  right: 12px;
  border: 0;
  background: transparent;
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
}
.name {
  margin: 4px 0;
  font-weight: 600;
}
.meta {
  margin: 4px 0;
  display: flex;
  gap: 12px;
  font-size: 12px;
  color: var(--zg-muted, #6b7280);
  flex-wrap: wrap;
}
.summary {
  margin: 6px 0;
  font-size: 13px;
  line-height: 1.6;
}
h4 {
  margin: 10px 0 6px;
  font-size: 14px;
}
.src {
  font-size: 12px;
  font-weight: 400;
  color: var(--zg-brand, #1d4ed8);
}
.warn {
  color: #b45309;
  font-size: 13px;
  margin: 6px 0;
}
.dim {
  color: var(--zg-muted, #6b7280);
  font-size: 12px;
}
ul {
  margin: 0;
  padding-left: 18px;
  font-size: 13px;
}
.risk p {
  margin: 4px 0;
  font-size: 13px;
}
.edit-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 12px;
  color: var(--zg-muted, #6b7280);
  margin-bottom: 6px;
}
.edit-row input {
  width: 84px;
  padding: 2px 4px;
  border: 1px solid var(--zg-line, #e5e7eb);
  border-radius: 4px;
}
.edit-row select {
  padding: 2px 4px;
  border: 1px solid var(--zg-line, #e5e7eb);
  border-radius: 4px;
}
.param {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.foot {
  position: sticky;
  bottom: 0;
  padding: 10px 0;
  background: var(--zg-panel, #fff);
  border-top: 1px solid var(--zg-line, #e5e7eb);
  margin-top: 12px;
}
.ops {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
</style>
