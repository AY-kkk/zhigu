<template>
  <div class="row">
    <!-- 连续放量快捷条件 -->
    <template v-if="node.kind === 'volume_increase'">
      <span class="text">最近连续 {{ node.days }} 个交易日成交量逐日增加（需 {{ node.days + 1 }} 根比较 bar）</span>
      <span v-if="node.repeat > 1" class="text dim">，连续 {{ node.repeat }} 次窗口满足</span>
      <label v-if="!readonly" class="edit">
        天数
        <input type="number" min="1" max="20" :value="node.days" @change="patch({ days: num($event) })">
        <button type="button" class="zg-btn zg-btn-ghost" @click="$emit('remove')">删除</button>
      </label>
    </template>
    <!-- 比较叶节点 -->
    <template v-else>
      <span class="text">{{ operandText(node.left) }} {{ opText }} {{ operandText(node.right) }}</span>
      <span v-if="node.repeat > 1" class="text dim">，连续 {{ node.repeat }} 个交易日满足（滞后 0～{{ node.repeat - 1 }} 逐项检查）</span>
      <span v-if="node.lag" class="text dim">（整体滞后 {{ node.lag }} 日）</span>
      <label v-if="!readonly" class="edit">
        值
        <input
          v-if="isConstant(node.right)"
          type="text"
          :value="constantOf(node.right)"
          @change="patchConstant($event.target.value)"
        >
        <input v-else type="text" :value="operandText(node.right)" disabled>
        持续
        <input type="number" min="1" max="20" :value="node.repeat || 1" @change="patch({ repeat: num($event) })">
        <button type="button" class="zg-btn zg-btn-ghost" @click="$emit('remove')">删除</button>
      </label>
    </template>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  node: { type: Object, required: true },
  readonly: { type: Boolean, default: true }
})
const emit = defineEmits(['update:node', 'remove'])

const opText = computed(
  () =>
    ({
      gt: '高于',
      gte: '不低于',
      lt: '低于',
      lte: '不高于',
      eq: '等于',
      crosses_above: '上穿',
      crosses_below: '下穿'
    }[props.node.op] || props.node.op)
)

function operandText(op) {
  if (typeof op === 'string') return op
  if (op && op.constant != null) return `常数 ${op.constant}`
  if (op && op.ref) return op.lag ? `${op.ref}[滞后 ${op.lag} 日]` : op.ref
  return '（缺失）'
}
function isConstant(op) {
  return Boolean(op && typeof op === 'object' && op.constant != null)
}
function constantOf(op) {
  return String(op.constant)
}
function num(e) {
  const n = Number(e.target.value)
  return Number.isFinite(n) ? n : 1
}
function patch(part) {
  emit('update:node', { ...props.node, ...part })
}
function patchConstant(value) {
  emit('update:node', { ...props.node, right: { constant: String(value) } })
}
</script>

<style scoped>
.row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border: 1px solid var(--zg-line, #e5e7eb);
  border-radius: 8px;
  margin-bottom: 6px;
  font-size: 13px;
}
.text.dim {
  color: var(--zg-muted, #6b7280);
}
.edit {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
  font-size: 12px;
  color: var(--zg-muted, #6b7280);
}
.edit input {
  width: 64px;
  padding: 2px 4px;
  border: 1px solid var(--zg-line, #e5e7eb);
  border-radius: 4px;
}
</style>
