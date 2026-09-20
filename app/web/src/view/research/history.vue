<template>
  <div>
    <h2>我的研究</h2>
    <ul>
      <li v-for="item in items" :key="item.run_id">
        <router-link :to="`/app/research/${item.run_id}`">{{ item.run_id }}</router-link>
        {{ item.status }} · {{ item.instrument_id }} · {{ item.mode }}
      </li>
    </ul>
    <el-button v-if="cursor" :disabled="busy" @click="more">加载更多</el-button>
    <p v-if="!items.length">暂无记录</p>
    <p v-if="error" class="err">{{ error }}</p>
  </div>
</template>
<script setup>
import { onMounted, ref } from 'vue'
import { listResearch } from '../../api/research.js'
const items = ref([])
const cursor = ref('')
const busy = ref(false)
const error = ref('')
async function load(reset) {
  busy.value = true
  try {
    const res = await listResearch({ limit: 20, cursor: reset ? '' : cursor.value })
    const page = res.data.items || []
    items.value = reset ? page : items.value.concat(page)
    cursor.value = res.data.next_cursor || ''
    error.value = ''
  } catch (e) {
    error.value = e.message || '网络中断，可重试'
  } finally {
    busy.value = false
  }
}
function more() { load(false) }
onMounted(() => load(true))
</script>
<style scoped>
.err { color: #c45656; }
</style>
