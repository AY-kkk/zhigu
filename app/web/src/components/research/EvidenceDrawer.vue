<template>
  <SideSheet
    className="zhigu-consumer zg-evidence-drawer"
    :title="drawerTitle"
    :visible="open"
    :width="width"
    placement="right"
    :closable="true"
    :closeIcon="closeIcon"
    :closeOnEsc="true"
    :maskClosable="true"
    :zIndex="210"
    :getPopupContainer="getConsumerPopupContainer"
    aria-label="证据详情"
    :onCancel="onClose"
  >
    <div v-if="error" class="body">
      <ResearchErrorState title="证据加载失败" :message="error" retry-label="重试加载" @retry="$emit('retry')" />
    </div>
    <div v-else-if="busy || !evidence" class="body" aria-busy="true">
      <p>正在加载证据</p>
      <div class="zg-skeleton">
        <div class="zg-skeleton-line" />
        <div class="zg-skeleton-line" />
        <div class="zg-skeleton-line" />
      </div>
    </div>
    <div v-else-if="!hasBody" class="body">
      <p>证据内容暂不可用</p>
      <button type="button" class="zg-btn zg-btn-secondary" @click="$emit('retry')">重试加载</button>
    </div>
    <div v-else class="body">
      <h3>{{ emptyField(evidence.title) }}</h3>
      <EvidenceTag v-if="relationLabel" :kind="relationKind" :label="relationLabel" />
      <p class="meta">{{ sourceKindLabel(evidence.source_kind) }}</p>
      <Tabs :activeKey="tab" :onChange="(key) => tab = key">
        <TabPane itemKey="text" tab="原文内容">
          <blockquote class="source-excerpt"><ZhiguIcon name="quote" :size="24" /><p>{{ emptyField(evidence.text) }}</p></blockquote>
          <p class="meta">定位：{{ emptyField(evidence.locator) }}</p>
          <p>外部原文：<SafeSourceLink :href="evidence.source_url" /></p>
          <button type="button" class="zg-btn zg-btn-secondary" @click="copyText">复制原文</button>
        </TabPane>
        <TabPane itemKey="meta" tab="关键信息">
          <p>披露时间：{{ formatDateTime(evidence.published_at) }}</p>
          <p>可获得时间：{{ formatDateTime(evidence.available_at) }}</p>
          <p>获取时间：{{ formatDateTime(evidence.retrieved_at) }}</p>
          <p>数据版本：{{ emptyField(evidence.data_version) }}</p>
          <p>数据模式：{{ modeLabel(evidence.mode) || '暂未提供' }}</p>
        </TabPane>
        <TabPane itemKey="data" tab="相关数据">
          <div v-if="metrics.length" class="metrics">
            <table>
              <thead>
                <tr>
                  <th>指标</th>
                  <th>数值</th>
                  <th>单位</th>
                  <th>期间</th>
                  <th>类型</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(m, i) in metrics" :key="i">
                  <td>{{ emptyField(m.metric) }}</td>
                  <td>{{ emptyField(m.value) }}</td>
                  <td>{{ emptyField(m.unit) }}</td>
                  <td>{{ formatDate(m.period_start) }}–{{ formatDate(m.period_end) }}</td>
                  <td>{{ m.value_type === 'actual' ? '实际' : m.value_type === 'estimate' ? '估计' : emptyField(m.value_type) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else>此证据未提供结构化指标</p>
        </TabPane>
      </Tabs>
    </div>
  </SideSheet>
</template>
<script setup>
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { SideSheet, TabPane, Tabs, Toast } from '@kousum/semi-ui-vue'
import { IconClose } from '@kousum/semi-icons-vue'
import SafeSourceLink from './SafeSourceLink.vue'
import EvidenceTag from './EvidenceTag.vue'
import ZhiguIcon from '../brand/ZhiguIcon.vue'
import ResearchErrorState from './ResearchErrorState.vue'
import { emptyField, formatDate, formatDateTime, modeLabel, sourceKindLabel } from '../../utils/researchCopy.js'
import { getConsumerPopupContainer } from '../../utils/popup.js'

const props = defineProps({
  open: Boolean,
  evidence: Object,
  error: { type: String, default: '' },
  busy: { type: Boolean, default: false },
  relation: { type: String, default: '' },
  index: { type: Number, default: 0 }
})
const emit = defineEmits(['close', 'retry'])
// Semi 的 SideSheet 关闭按钮没有可访问名；用带中文标签的图标补上。
const closeIcon = h(IconClose, { 'aria-label': '关闭证据详情' })
const viewport = ref(typeof window === 'undefined' ? 1200 : window.innerWidth)
const tab = ref('text')
function onResize() { viewport.value = window.innerWidth }
onMounted(() => window.addEventListener('resize', onResize))
onUnmounted(() => window.removeEventListener('resize', onResize))
watch(() => props.open, (open) => { if (open) tab.value = 'text' })
const drawerTitle = computed(() => (props.index ? `证据详情 · [${props.index}]` : '证据详情'))
const width = computed(() => {
  if (viewport.value < 768) return '100%'
  if (viewport.value < 1024) return 440
  if (viewport.value < 1200) return 480
  return 520
})
const metrics = computed(() => Array.isArray(props.evidence?.metrics) ? props.evidence.metrics : [])
const hasBody = computed(() => !!(props.evidence?.text || props.evidence?.title))
const relationKind = computed(() => (props.relation === 'challenge' ? 'challenged' : props.relation === 'support' ? 'supported' : 'mixed'))
const relationLabel = computed(() => {
  if (props.relation === 'both') return '同时关联支持与挑战'
  if (props.relation === 'support') return '支持证据'
  if (props.relation === 'challenge') return '反方证据'
  return ''
})
function onClose() { emit('close') }
async function copyText() {
  const text = props.evidence?.text || ''
  try {
    await navigator.clipboard.writeText(text)
    Toast.info({ content: '原文已复制', duration: 2 })
  } catch {
    Toast.error({ content: '复制失败' })
  }
}
</script>
<style scoped>
.head { display: flex; align-items: flex-start; gap: 8px; }
.kicker { margin: 0 0 6px; font-weight: 600; }
.body { display: grid; gap: 16px; overflow-wrap: anywhere; word-break: break-word; }
h3 { margin: 0; font-family: var(--zg-font-editorial); font-size: 22px; line-height: 34px; }
.meta { color: var(--zg-text-secondary); font-size: 12px; line-height: 18px; }
.source-excerpt {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  word-break: break-word;
  max-width: 100%;
  background: var(--zg-paper);
  padding: 24px;
  margin: 20px 0;
  border-left: 2px solid var(--zg-line);
  font-family: var(--zg-font-body);
  font-size: 16px;
  line-height: 28px;
}
.source-excerpt :deep(.zg-icon) { color: var(--zg-brand); margin-bottom: 12px; }
.source-excerpt p { margin: 0; }
.body :deep(.semi-tabs-content) { padding-top: 8px; }
.body :deep(.semi-tabs-tab) { font-size: 14px; }
.metrics { overflow-x: auto; }
table { width: 100%; border-collapse: collapse; font-size: 12px; }
th, td { border-bottom: 1px solid var(--zg-line); padding: 8px; text-align: left; }
</style>
