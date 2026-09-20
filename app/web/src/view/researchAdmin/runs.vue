<template>
  <div>
    <h2>运行记录（脱敏）</h2>
    <p>默认不展示完整观点或对话。</p>
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
.err { color: #c45656; }
li { overflow-wrap: anywhere; }
</style>
