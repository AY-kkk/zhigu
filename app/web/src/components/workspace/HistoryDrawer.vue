<template>
  <SideSheet
    class="zhigu-consumer"
    title="历史记录"
    :visible="open"
    :width="width"
    placement="right"
    :closable="true"
    :closeOnEsc="true"
    :getPopupContainer="getConsumerPopupContainer"
    :onCancel="$emit('close')"
  >
    <ResearchHistoryList
      :items="items"
      :cursor="cursor"
      :busy="busy"
      :error="error"
      @more="$emit('more')"
      @open="onOpen"
      @start="onStart"
    />
    <Button theme="borderless" @click="onAll">查看全部</Button>
  </SideSheet>
</template>
<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Button, SideSheet } from '@kousum/semi-ui-vue'
import ResearchHistoryList from '../research/ResearchHistoryList.vue'
import { getConsumerPopupContainer } from '../../utils/popup.js'

defineProps({
  open: Boolean,
  items: { type: Array, default: () => [] },
  cursor: { type: String, default: '' },
  busy: { type: Boolean, default: false },
  error: { type: String, default: '' }
})
const emit = defineEmits(['close', 'more', 'open', 'start'])
const router = useRouter()
const viewport = ref(typeof window === 'undefined' ? 1200 : window.innerWidth)
function onResize() { viewport.value = window.innerWidth }
onMounted(() => window.addEventListener('resize', onResize))
onUnmounted(() => window.removeEventListener('resize', onResize))
const width = computed(() => (viewport.value < 768 ? '100%' : 480))
function onOpen(id) {
  emit('open', id)
  emit('close')
  router.push(`/app/research/${id}`)
}
function onStart() {
  emit('start')
  emit('close')
  router.push('/app/research/new')
}
function onAll() {
  emit('close')
  router.push('/app/profile?tab=history')
}
</script>
