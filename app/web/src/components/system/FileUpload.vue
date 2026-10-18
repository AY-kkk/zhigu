<script setup>
import { computed, onBeforeUnmount, ref } from 'vue'

const props = defineProps({
  files: { type: Array, default: () => [] },
  accept: { type: String, default: '.pdf,.txt,.md' },
  maxBytes: { type: Number, default: 20 * 1024 * 1024 },
  disabled: { type: Boolean, default: false },
})

const emit = defineEmits(['select', 'cancel', 'retry'])
const input = ref(null)
const dragging = ref(false)
const objectUrls = new Set()

const limitText = computed(() => `支持 ${props.accept}，单文件最大 ${Math.floor(props.maxBytes / 1024 / 1024)} MiB`)

function openPicker() {
  if (!props.disabled) input.value?.click()
}

function choose(files) {
  const file = files?.[0]
  if (!file) return
  if (file.size > props.maxBytes) {
    emit('select', {
      file,
      rejected: true,
      reason: `文件超过 ${Math.floor(props.maxBytes / 1024 / 1024)} MiB`,
    })
    return
  }
  emit('select', { file, rejected: false })
}

function onDrop(event) {
  dragging.value = false
  if (!props.disabled) choose(event.dataTransfer?.files)
}

onBeforeUnmount(() => {
  for (const url of objectUrls) URL.revokeObjectURL(url)
  objectUrls.clear()
})
</script>

<template>
  <section class="file-upload">
    <div
      class="drop-zone"
      :class="{ dragging, disabled }"
      @dragenter.prevent="dragging = true"
      @dragover.prevent
      @dragleave.prevent="dragging = false"
      @drop.prevent="onDrop"
    >
      <input
        ref="input"
        class="sr-only"
        type="file"
        :accept="accept"
        :disabled="disabled"
        aria-describedby="file-upload-limit"
        @change="choose($event.target.files); $event.target.value = ''"
      >
      <button type="button" :disabled="disabled" @click="openPicker">选择材料</button>
      <p id="file-upload-limit">{{ limitText }}</p>
    </div>
    <ul v-if="files.length" class="file-list">
      <li v-for="item in files" :key="item.id">
        <div>
          <strong>{{ item.name }}</strong>
          <small>{{ Math.ceil((item.size || 0) / 1024) }} KiB · {{ item.status }}</small>
          <small v-if="item.error" role="alert">{{ item.error }}</small>
          <progress v-if="item.status === 'uploading'" :value="item.progress || 0" max="100" />
        </div>
        <button v-if="item.status === 'uploading' || item.status === 'queued'" type="button" @click="$emit('cancel', item.id)">取消</button>
        <button v-else-if="item.status === 'failed'" type="button" @click="$emit('retry', item.id)">重试此文件</button>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.drop-zone {
  display: flex;
  align-items: center;
  gap: 14px;
  min-height: 88px;
  padding: 16px;
  border: 1px dashed var(--zg-line, #8c858f);
  border-radius: 10px;
}
.drop-zone.dragging { border-color: var(--zg-action, #b4232c); background: color-mix(in srgb, var(--zg-action, #b4232c) 6%, white); }
.drop-zone.disabled { opacity: .55; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0, 0, 0, 0); }
button { min-height: 40px; padding: 8px 13px; border: 1px solid currentColor; border-radius: 8px; }
.file-list { display: grid; gap: 10px; padding: 0; list-style: none; }
.file-list li { display: flex; justify-content: space-between; gap: 12px; padding: 12px; border: 1px solid var(--zg-line, #dedbd4); }
.file-list small { display: block; color: var(--zg-muted, #68616b); }
</style>
