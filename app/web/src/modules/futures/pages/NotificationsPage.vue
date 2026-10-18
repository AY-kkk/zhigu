<template>
  <section class="notifications-page">
    <PageHeader
      title="期货通知"
      summary="筛选状态映射到服务端字段；批量已读失败时恢复所有受影响行。"
      :primary-action="{ label: '批量已读', disabled: !selectedIds.length || busy }"
      @primary="batchRead"
    />

    <form class="filters" @submit.prevent="load">
      <label>状态
        <select v-model="filter.read">
          <option :value="undefined">全部</option>
          <option :value="false">未读</option>
          <option :value="true">已读</option>
        </select>
      </label>
      <label>类型
        <select v-model="filter.type">
          <option value="">全部</option>
          <option v-for="type in notificationTypes" :key="type" :value="type">{{ type }}</option>
        </select>
      </label>
      <label>对象 ID<input v-model.trim="filter.object_id" placeholder="可选"></label>
      <label>对象版本<input v-model.number="filter.object_version" type="number" min="1"></label>
      <button type="submit">应用筛选</button>
    </form>

    <RequestError v-if="error" :error="error" @retry="load" />
    <LoadingState v-if="loading" label="正在读取通知" :rows="5" />
    <EmptyState
      v-else-if="!items.length"
      title="没有符合条件的通知"
      description="筛选条件会传给服务端；这里不会用本地空数组冒充服务端结果。"
    />

    <ul v-else class="notification-list">
      <li v-for="item in items" :key="item.id" :class="{ unread: !item.read_at }">
        <label>
          <input
            v-model="selectedIds"
            type="checkbox"
            :value="item.id"
            :aria-label="`选择通知 ${item.id}`"
          >
          <span>
            <strong>{{ label(item.type) }}</strong>
            <small>{{ item.created_at }} · {{ item.object_id }} · 版本 {{ item.object_version }}</small>
            <small>{{ severity(item.type) }} · {{ item.reason || '服务端未提供扩展原因' }}</small>
          </span>
        </label>
        <RouterLink :to="targetFor(item)">定位上下文</RouterLink>
      </li>
    </ul>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import EmptyState from '../../../components/system/EmptyState.vue'
import LoadingState from '../../../components/system/LoadingState.vue'
import RequestError from '../../../components/system/RequestError.vue'
import PageHeader from '../../../components/workspace/PageHeader.vue'
import { useFuturesStore } from '../store.js'

const store = useFuturesStore()
const filter = ref({ read: undefined, type: '', object_id: '', object_version: null })
const selectedIds = ref([])
const loading = ref(true)
const busy = ref(false)
const error = ref(null)
const notificationTypes = [
  'check_changed', 'source_unavailable', 'revision', 'retraction',
  'expired', 'run_succeeded', 'run_failed',
]
const items = computed(() => store.notifications)
const filterPayload = computed(() => Object.fromEntries(Object.entries(filter.value).filter(([, value]) => value !== '' && value !== null && value !== undefined)))

function label(type) {
  return ({
    check_changed: '条件检查变化',
    source_unavailable: '来源不可用',
    revision: '证据修订',
    retraction: '证据撤回',
    expired: '假设到期',
    run_succeeded: '研究完成',
    run_failed: '研究失败',
  })[type] || type
}

function severity(type) {
  return ['run_failed', 'source_unavailable', 'retraction'].includes(type) ? '高优先级' : '普通'
}

function targetFor(item) {
  if (item.type?.startsWith('run_')) return `/app/futures/research/${encodeURIComponent(item.object_id)}`
  if (item.type === 'check_changed' || item.type === 'expired') return `/app/futures/hypotheses/${encodeURIComponent(item.object_id)}`
  return '/app/futures'
}

async function load() {
  loading.value = true
  error.value = null
  try {
    await store.loadNotifications(filterPayload.value)
    selectedIds.value = []
  } catch (reason) {
    error.value = reason
  } finally {
    loading.value = false
  }
}

async function batchRead() {
  if (!selectedIds.value.length || selectedIds.value.length > 100) return
  busy.value = true
  try {
    await store.batchReadNotifications(selectedIds.value)
    selectedIds.value = []
  } catch (reason) {
    error.value = reason
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.filters { display: grid; grid-template-columns: repeat(4, minmax(140px, 1fr)) auto; gap: 12px; align-items: end; margin-bottom: 20px; }
.filters label { display: grid; gap: 5px; }
.filters input, .filters select, .filters button { min-height: 40px; padding: 8px 10px; border: 1px solid var(--zg-line, #dedbd4); border-radius: 8px; }
.notification-list { display: grid; gap: 0; padding: 0; list-style: none; }
.notification-list li { display: flex; justify-content: space-between; gap: 14px; padding: 15px 12px; border-bottom: 1px solid var(--zg-line, #dedbd4); }
.notification-list li.unread { border-inline-start: 4px solid var(--zg-action, #b4232c); }
.notification-list label { display: flex; gap: 10px; }
.notification-list small { display: block; color: var(--zg-muted, #68616b); }
@media (max-width: 767px) {
  .filters { grid-template-columns: 1fr 1fr; }
  .notification-list li { align-items: flex-start; flex-direction: column; }
}
</style>
