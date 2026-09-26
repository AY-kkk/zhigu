<template>
  <fieldset class="grp" :class="{ nested: depth > 0 }">
    <legend class="label">{{ labelText }}<span v-if="depth > 0" class="dim">（分组）</span></legend>
    <template v-for="(child, i) in children" :key="i">
      <!-- 条件组递归 -->
      <RuleGroup
        v-if="isGroup(child)"
        :node="child"
        :readonly="readonly"
        :depth="depth + 1"
        @update:node="replaceAt(i, $event)"
        @remove="removeAt(i)"
      />
      <!-- 叶节点 / 快捷条件 -->
      <RuleConditionRow
        v-else
        :node="child"
        :readonly="readonly"
        @update:node="replaceAt(i, $event)"
        @remove="removeAt(i)"
      />
    </template>
    <button v-if="!readonly" type="button" class="zg-btn zg-btn-ghost" @click="addLeaf">+ 添加条件</button>
  </fieldset>
</template>

<script setup>
import { computed } from 'vue'
import RuleConditionRow from './RuleConditionRow.vue'

const props = defineProps({
  node: { type: Object, required: true },
  readonly: { type: Boolean, default: true },
  depth: { type: Number, default: 0 }
})
const emit = defineEmits(['update:node', 'remove'])

const kind = computed(() => (props.node.all ? 'all' : props.node.any ? 'any' : 'not'))
const children = computed(() => {
  if (kind.value === 'not') return [props.node.not]
  return props.node[kind.value] || []
})
const labelText = computed(() => ({ all: '同时满足以下条件', any: '满足任一条件', not: '取反' }[kind.value]))

function isGroup(node) {
  return Boolean(node && typeof node === 'object' && (node.all || node.any || node.not))
}
function replaceAt(i, next) {
  if (kind.value === 'not') {
    emit('update:node', { not: next })
    return
  }
  const list = children.value.map((c, idx) => (idx === i ? next : c))
  emit('update:node', { [kind.value]: list })
}
function removeAt(i) {
  if (kind.value === 'not') {
    emit('remove')
    return
  }
  const list = children.value.filter((_, idx) => idx !== i)
  if (!list.length) {
    emit('remove')
    return
  }
  emit('update:node', { [kind.value]: list })
}
function addLeaf() {
  const leaf = { op: 'lt', left: 'kdj.j', right: { constant: '20' } }
  if (kind.value === 'not') return
  emit('update:node', { [kind.value]: [...children.value, leaf] })
}
</script>

<style scoped>
.grp {
  border: 1px dashed var(--zg-line, #cbd5e1);
  border-radius: 10px;
  padding: 8px 10px;
  margin-bottom: 8px;
}
.grp.nested {
  margin-left: 14px;
}
.label {
  font-size: 13px;
  font-weight: 600;
  padding: 0 4px;
}
.dim {
  color: var(--zg-muted, #6b7280);
  font-weight: 400;
}
</style>
