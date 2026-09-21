<template>
  <div class="zhigu-page">
    <h1>运行记录</h1>
    <p>脱敏列表，默认不展示完整观点或对话。</p>
    <p v-if="error" class="err">{{ error }}</p>
    <ul>
      <li v-for="r in items" :key="r.run_id">
        {{ r.run_id }} · {{ r.user_id || 'user_unknown' }} · {{ r.stage }} / {{ r.status }}
        · {{ r.duration_ms || 0 }}ms · 用量 {{ r.usage_tokens || 0 }}
        <span v-if="r.error_code"> · {{ r.error_code }}</span>
      </li>
    </ul>
  </div>
</template>
<script setup>
import { onMounted, ref } from 'vue'
import http from '../../utils/http.js'
const items = ref([])
const error = ref('')
onMounted(async () => {
  try {
    const res = await http.get('/api/finance/admin/research-runs')
    items.value = res.data.items || []
  } catch (e) {
    items.value = []
    error.value = e.message || '无法加载运行记录'
  }
})
</script>
<style scoped>
.err { color: var(--zg-error-fg); background: var(--zg-error-bg); padding: 8px 12px; border-radius: var(--zg-radius-control); }
ul { margin: 0; padding: 0; list-style: none; }
li {
  overflow-wrap: anywhere;
  color: var(--zg-ink);
  font-family: var(--zg-font-number);
  font-size: 13px;
  line-height: 22px;
  padding: 12px 0;
  border-bottom: 1px solid var(--zg-line);
}
li:last-child { border-bottom: none; }
</style>
