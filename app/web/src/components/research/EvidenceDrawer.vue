<template>
  <SideSheet
    class="zhigu-consumer"
    title="证据详情"
    :visible="open"
    :width="width"
    placement="right"
    :closable="true"
    :closeOnEsc="true"
    :maskClosable="true"
    :getPopupContainer="getConsumerPopupContainer"
    aria-label="证据详情"
    :onCancel="onClose"
  >
    <div v-if="error" class="body">
      <p class="err">{{ error }}</p>
      <Button @click="$emit('retry')">重试加载</Button>
    </div>
    <div v-else-if="evidence" class="body">
      <section>
        <h3>来源</h3>
        <p><strong>{{ emptyField(evidence.title) }}</strong></p>
        <p>来源名称：{{ emptyField(evidence.source_kind || evidence.source_name) }}</p>
        <p>外部原文：<SafeSourceLink :href="evidence.source_url" /></p>
      </section>
      <section>
        <h3>定位</h3>
        <p>{{ emptyField(evidence.locator) }}</p>
      </section>
      <section>
        <h3>原文</h3>
        <pre>{{ emptyField(evidence.text) }}</pre>
        <Button size="small" @click="copyText">复制原文</Button>
      </section>
      <section>
        <h3>时间</h3>
        <p>披露时间：{{ formatDateTime(evidence.published_at) }}</p>
        <p>可获得时间：{{ formatDateTime(evidence.available_at) }}</p>
        <p>获取时间：{{ formatDateTime(evidence.retrieved_at) }}</p>
      </section>
      <section>
        <h3>指标</h3>
        <ul v-if="metrics.length">
          <li v-for="(m, i) in metrics" :key="i">
            {{ emptyField(m.metric) }}
            {{ emptyField(m.value) }} {{ emptyField(m.unit) }}
            · {{ emptyField(m.currency) }}
            · {{ emptyField(m.period_start) }}–{{ emptyField(m.period_end) }}
            · {{ emptyField(m.value_type) }}
          </li>
        </ul>
        <p v-else>暂未提供</p>
      </section>
      <section>
        <h3>数据记录</h3>
        <p>版本：{{ emptyField(evidence.data_version) }}</p>
        <p>标识：{{ modeLabel(evidence.mode) || emptyField(evidence.mode) }}</p>
      </section>
    </div>
    <p v-else>正在打开证据…</p>
  </SideSheet>
</template>
<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Button, SideSheet, Toast } from '@kousum/semi-ui-vue'
import SafeSourceLink from './SafeSourceLink.vue'
import { emptyField, formatDateTime, modeLabel } from '../../utils/researchCopy.js'
import { getConsumerPopupContainer } from '../../utils/popup.js'

const props = defineProps({
  open: Boolean,
  evidence: Object,
  error: { type: String, default: '' }
})
const emit = defineEmits(['close', 'retry'])
const viewport = ref(typeof window === 'undefined' ? 1200 : window.innerWidth)
function onResize() { viewport.value = window.innerWidth }
onMounted(() => window.addEventListener('resize', onResize))
onUnmounted(() => window.removeEventListener('resize', onResize))
const width = computed(() => (viewport.value < 768 ? '100%' : 480))
const metrics = computed(() => {
  const raw = props.evidence?.metrics
  return Array.isArray(raw) ? raw : []
})
function onClose() { emit('close') }
async function copyText() {
  const text = props.evidence?.text || ''
  try {
    await navigator.clipboard.writeText(text)
    Toast.success({ content: '原文已复制', duration: 2 })
  } catch {
    Toast.error({ content: '复制失败' })
  }
}
</script>
<style scoped>
.body { display: grid; gap: 20px; overflow-wrap: anywhere; word-break: break-word; }
h3 { margin: 0 0 8px; font-size: 16px; line-height: 24px; }
pre { white-space: pre-wrap; overflow-wrap: anywhere; word-break: break-word; max-width: 100%; background: #f6f4f4; padding: 12px; border-radius: 12px; }
.err { color: var(--zg-danger); }
</style>
