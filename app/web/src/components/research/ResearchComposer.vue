<template>
  <div class="composer zhigu-composer" :class="{ disabled: disabled }">
    <p v-if="hint" class="hint">{{ hint }}</p>
    <TextArea
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      :autosize="{ minRows: 3, maxRows: 6 }"
      :getValueLength="countChars"
      aria-label="研究输入"
      :onChange="onInput"
      @compositionstart="onComposeStart"
      @compositionend="onComposeEnd"
      @keydown="onKeydown"
    />
    <div class="bar">
      <span class="count" :class="{ warn: tooShort || tooLong }">{{ count }}/2000</span>
      <span v-if="error" class="err" role="status">{{ error }}</span>
      <Button
        type="primary"
        theme="solid"
        :loading="busy"
        :disabled="!canSubmit || disabled"
        @click="$emit('submit')"
      >
        {{ actionLabel }}
      </Button>
    </div>
  </div>
</template>
<script setup>
import { computed, ref } from 'vue'
import { Button, TextArea } from '@kousum/semi-ui-vue'
import { countChars } from '../../utils/researchCopy.js'

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  hint: { type: String, default: '' },
  error: { type: String, default: '' },
  actionLabel: { type: String, default: '解析观点' },
  disabled: { type: Boolean, default: false },
  busy: { type: Boolean, default: false },
  minChars: { type: Number, default: 20 }
})
const emit = defineEmits(['update:modelValue', 'submit'])
const composing = ref(false)
const count = computed(() => countChars(props.modelValue))
const tooShort = computed(() => count.value < props.minChars)
const tooLong = computed(() => count.value > 2000)
const canSubmit = computed(() => !tooShort.value && !tooLong.value && !props.busy && !props.disabled)

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
    if (canSubmit.value) emit('submit')
    return
  }
}
</script>
<style scoped>
.composer {
  width: 100%;
  max-width: var(--zg-reading-max);
  margin: 0 auto;
  padding: 12px 32px 20px;
  background: var(--semi-color-bg-1);
}
.hint {
  margin: 0 0 8px;
  color: var(--semi-color-text-1);
  font-size: 12px;
  line-height: 18px;
}
.bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 8px;
}
.count { color: var(--semi-color-text-1); font-size: 12px; margin-right: auto; }
.count.warn { color: var(--zg-danger); }
.err { color: var(--zg-danger); font-size: 12px; }
.disabled { background: #f6f4f4; }
@media (max-width: 767px) {
  .composer { padding: 12px 16px calc(12px + env(safe-area-inset-bottom)); }
}
</style>
