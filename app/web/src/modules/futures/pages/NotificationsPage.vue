<template>
  <section class="futures-page">
    <p class="eyebrow">期货研究站内通知</p><h2>通知</h2>
    <p v-if="error" class="error" role="alert">{{ error.message }}</p>
    <ul class="notification-list">
      <li v-for="item in items" :key="item.id" :class="{ unread: !item.read_at }">
        <RouterLink :to="target(item)"><strong>{{ label(item.type) }}</strong><span>{{ item.object_id }}</span></RouterLink>
        <time>{{ item.created_at }}</time><button v-if="!item.read_at" @click="mark(item)">标为已读</button>
      </li>
      <li v-if="!items.length">没有期货研究通知。</li>
    </ul>
  </section>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { getNotifications, markNotification } from '../api.js'

const items = ref([])
const error = ref(null)
function label(type) { return ({ check_changed: '条件检查变化', source_unavailable: '来源不可用', revision: '证据修订', retraction: '证据撤回', expired: '假设到期', run_succeeded: '研究完成', run_failed: '研究失败' })[type] || type }
function target(item) { return item.type.startsWith('run_') ? `/app/futures/research/${item.object_id}` : `/app/futures/hypotheses/${item.object_id}` }
async function load() { try { items.value = (await getNotifications(undefined, 100)).items || [] } catch (failure) { error.value = failure } }
async function mark(item) { await markNotification(item.id); await load() }
onMounted(load)
</script>

<style scoped>
.eyebrow { color: var(--zg-action); }.notification-list { list-style: none; padding: 0; }.notification-list li { display: grid; grid-template-columns: 1fr auto auto; gap: 12px; align-items: center; padding: 14px 0; border-bottom: 1px solid #ddd; }.notification-list li.unread { border-left: 3px solid var(--zg-action); padding-left: 10px; }.notification-list a { color: inherit; }.error { color: #b42318; }
</style>
