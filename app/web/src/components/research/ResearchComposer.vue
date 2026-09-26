<template>
  <div class="composer zhigu-composer" :class="{ disabled, compact: minChars <= 1 }">
    <p v-if="hint" class="hint">{{ hint }}</p>
    <label v-if="fieldLabel" class="label" :for="inputId">{{ fieldLabel }}</label>
    <div class="input-surface" :class="{ 'has-error': showError }">
    <TextArea
      :id="inputId"
      :aria-label="fieldLabel || '研究输入'"
      ref="areaRef"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled || busy"
      :autosize="{ minRows: minRows, maxRows: 6 }"
      :getValueLength="countChars"
      :aria-invalid="showError ? 'true' : undefined"
      :aria-describedby="describedBy"
      :onChange="onInput"
      @compositionstart="onComposeStart"
      @compositionend="onComposeEnd"
      @keydown="onKeydown"
      @blur="touched = true"
    />
    <div class="bar">
      <span id="zg-charcount" class="count" :class="{ warn: showLimitError }" aria-live="off">{{ count }}/2000</span>
      <span v-if="!showError && !disabled" class="shortcut">⌘ / Ctrl + Enter</span>
      <span v-if="showError" :id="errorId" class="err" role="status">{{ visibleError }}</span>
      <button
        type="button"
        class="zg-btn zg-btn-primary"
        :disabled="!canSubmit || disabled"
        @click="$emit('submit')"
      >
        <span class="spinner" v-if="busy" aria-hidden="true" />
        {{ busyLabel }}
        <ZhiguIcon v-if="!busy && showArrow" name="arrow" :size="16" />
      </button>
    </div>
    </div>
  </div>
</template>
<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import { TextArea } from '@kousum/semi-ui-vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'
import { countChars } from '../../utils/researchCopy.js'

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  hint: { type: String, default: '' },
  error: { type: String, default: '' },
  actionLabel: { type: String, default: '解析观点' },
  fieldLabel: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  busy: { type: Boolean, default: false },
  minChars: { type: Number, default: 20 },
  showArrow: { type: Boolean, default: false },
  focusToken: { type: Number, default: 0 },
  coverageHint: { type: String, default: '' }
})
const emit = defineEmits(['update:modelValue', 'submit'])
const composing = ref(false)
const touched = ref(false)
const attempted = ref(false)
const areaRef = ref(null)
const inputId = 'zg-composer-input'
const errorId = 'zg-composer-error'
const count = computed(() => countChars(props.modelValue))
const tooShort = computed(() => count.value < props.minChars)
const tooLong = computed(() => count.value > 2000)
const showLimitError = computed(() => (touched.value || attempted.value) && (tooShort.value || tooLong.value))
const visibleError = computed(() => {
  if (props.error) return props.error
  if (!showLimitError.value) return ''
  if (count.value === 0 && props.minChars <= 1) return '请输入追问内容。'
  if (tooShort.value) return `请再写 ${props.minChars - count.value} 字，至少 ${props.minChars} 字。`
  if (tooLong.value) return '已超过 2000 字，请删减后再提交。'
  return ''
})
const showError = computed(() => !!visibleError.value)
const describedBy = computed(() => {
  const ids = ['zg-charcount']
  if (showError.value) ids.push(errorId)
  return ids.join(' ')
})
const canSubmit = computed(() => !tooShort.value && !tooLong.value && !props.busy && !props.disabled)
const busyLabel = computed(() => (props.busy ? '正在处理' : props.actionLabel))
const minRows = computed(() => (props.minChars <= 1 ? 1 : 3))

function onInput(value) {
  emit('update:modelValue', value)
}
function onComposeStart() { composing.value = true }
function onComposeEnd() { composing.value = false }
function onKeydown(event) {
  if (composing.value || event.isComposing) return
  if (event.key !== 'Enter') return
  if (event.metaKey || event.ctrlKey) {
    event.preventDefault()
    attempted.value = true
    if (canSubmit.value) emit('submit')
  }
}

watch(() => props.focusToken, async () => {
  await nextTick()
  const el = areaRef.value?.$el?.querySelector?.('textarea') || areaRef.value?.$el
  if (el && typeof el.focus === 'function') el.focus()
})

defineExpose({
  markAttempted() { attempted.value = true },
  focus() {
    const el = areaRef.value?.$el?.querySelector?.('textarea') || areaRef.value?.$el
    if (el && typeof el.focus === 'function') el.focus()
  }
})
</script>
<style scoped>
.composer {
  width: 100%;
  max-width: var(--zg-reading-max);
  margin: 0 auto;
  padding: 16px var(--zg-page-pad) calc(16px + env(safe-area-inset-bottom, 0px));
  background: var(--zg-paper);
}
.input-surface {
  border: 1px solid var(--zg-control-border);
  border-radius: 10px;
  background: var(--zg-surface);
  padding: 8px 12px 12px;
  transition: border-color var(--zg-duration-fast), box-shadow var(--zg-duration-fast);
}
.input-surface:focus-within { border-color: var(--zg-action); box-shadow: 0 0 0 3px rgba(184,58,61,.07); }
.input-surface.has-error { border-color: var(--zg-error-fg); }
.input-surface :deep(.semi-input-textarea-wrapper) { border: 0; background: transparent; box-shadow: none; }
.input-surface :deep(.semi-input-textarea) { background: transparent; padding: 10px 2px; font-size: 15px; line-height: 26px; box-shadow: none; border: 0; }
.shortcut { font-size: 11px; color: var(--zg-text-secondary); }
.label {
  display: block;
  margin: 0 0 8px;
  font-size: 14px;
  line-height: 22px;
  font-weight: 500;
}
.hint {
  margin: 0 0 8px;
  color: var(--zg-text-secondary);
  font-size: 12px;
  line-height: 18px;
}
.bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.count { color: var(--zg-text-secondary); font-size: 12px; margin-right: auto; font-variant-numeric: tabular-nums; }
.count.warn { color: var(--zg-error-fg); }
.err { color: var(--zg-error-fg); font-size: 12px; }
.disabled .input-surface { background: var(--zg-surface-muted); border-color: var(--zg-line); }
.compact { display: flex; flex-direction: column; padding-top: 12px; }
.compact .label, .compact .count { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
.compact .input-surface { display: flex; align-items: flex-end; gap: 8px; padding: 8px 10px; }
.compact .input-surface :deep(.semi-input-textarea-wrapper) { min-width: 0; flex: 1; }
.compact .bar { flex: 0 0 auto; margin: 0; }
.compact .shortcut { display: none; }
.compact .hint { order: 2; margin: 8px 0 0; }
.compact .err { max-width: 140px; }
@media (max-width: 767px) {
  .shortcut { display: none; }
  .input-surface { padding: 6px 10px 10px; }
  .input-surface :deep(.semi-input-textarea) { font-size: 16px; }
  .compact .bar .zg-btn { padding: 0 10px; }
  .compact .input-surface :deep(.semi-input-textarea) { font-size: 14px; }
}
.spinner {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255,255,255,.4);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 900ms linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) {
  .spinner { animation: none; }
}
</style>
