<template>
  <div class="document-upload">
    <div class="link-row">
      <input v-model="urlValue" class="url-input" type="url" inputmode="url" placeholder="粘贴微信公众号或其他可读网页链接" :disabled="busy" aria-label="网页链接">
      <button type="button" class="zg-btn zg-btn-secondary" :disabled="busy || !urlValue.trim()" @click="submitLink">读取链接</button>
    </div>
    <input
      id="research-document-input"
      ref="fileRef"
      class="file-input"
      type="file"
      accept=".pdf,.docx,.txt,application/pdf,text/plain"
      :disabled="busy"
      @change="onFile"
    >
    <label for="research-document-input" class="upload-label">
      <ZhiguIcon name="report" :size="16" />
      上传研报
    </label>
    <div v-if="document" class="document-chip">
      <span>{{ document.filename }}</span>
      <span class="status">{{ statusText }}</span>
      <button type="button" class="remove" :disabled="busy" @click="$emit('remove')">移除</button>
    </div>
    <p v-if="error" class="error" role="status">{{ error }}</p>
    <p v-else class="hint">支持 PDF、DOCX、TXT 上传，或粘贴微信公众号等公开网页链接；材料可同时作为证据和被质证对象。</p>
  </div>
</template>
<script setup>
import { computed, ref } from 'vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'

const props = defineProps({
  document: { type: Object, default: null },
  busy: Boolean,
  error: { type: String, default: '' }
})
const emit = defineEmits(['upload', 'submit-link', 'remove'])
const urlValue = ref('')
const fileRef = ref(null)
const statusText = computed(() => {
  const status = props.document?.extraction_status
  if (status === 'succeeded') return '解析完成'
  if (status === 'failed') return '解析失败'
  return '解析中'
})
function submitLink() {
  const value = urlValue.value.trim()
  if (value) emit('submit-link', value)
}
function onFile(event) {
  const file = event.target.files?.[0]
  if (file) emit('upload', file)
  event.target.value = ''
}
</script>
<style scoped>
.document-upload { display: grid; gap: 6px; padding: 8px 0 0; }
.link-row { display: flex; gap: 8px; }
.url-input { flex: 1; min-width: 0; min-height: 36px; padding: 6px 10px; border: 1px solid var(--zg-line); border-radius: 8px; }
.file-input { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
.upload-label { justify-self: start; display: inline-flex; align-items: center; gap: 6px; min-height: 36px; padding: 6px 12px; border: 1px solid var(--zg-line); border-radius: 8px; cursor: pointer; color: var(--zg-action); background: var(--zg-surface); }
.document-chip { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; font-size: 13px; }
.status { color: var(--zg-text-secondary); }
.remove { border: 0; background: transparent; color: var(--zg-action); cursor: pointer; }
.hint, .error { margin: 0; font-size: 12px; line-height: 18px; }
.hint { color: var(--zg-text-secondary); }
.error { color: var(--zg-danger, #b42318); }
</style>
