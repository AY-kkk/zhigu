<template>
  <main class="admin-sources">
    <p class="eyebrow">管理员 / 期货数据</p><h1>来源与运行控制</h1>
    <section><h2>运行模式</h2><select v-model="operation.mode"><option value="off">off</option><option value="read_only">read_only</option><option value="live">live</option></select><input v-model="operation.reason" placeholder="变更原因"><button @click="saveOperations">提交运行变更</button></section>
    <section><h2>来源</h2><button @click="loadSources">刷新</button><ul><li v-for="source in sources" :key="source.id">{{ source.publisher }} / {{ source.status }} / {{ source.health }}</li></ul></section>
    <section><h2>新增来源清单</h2><textarea v-model="manifestText" aria-label="Source manifest JSON"></textarea><button @click="submitSource">登记来源</button></section>
    <p v-if="error" class="error" role="alert">{{ error.message }}</p>
  </main>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { createSource, getOperations, getSources, patchOperations } from '../api.js'

const sources = ref([])
const operation = ref({ expected_version: 1, mode: 'off', reason: '', allowed_user_ids: [] })
const manifestText = ref('{}')
const error = ref(null)
async function loadSources() { try { sources.value = (await getSources(undefined, 100)).items || [] } catch (failure) { error.value = failure } }
async function saveOperations() { try { operation.value = await patchOperations(operation.value) } catch (failure) { error.value = failure } }
async function submitSource() { try { await createSource(JSON.parse(manifestText.value)); await loadSources() } catch (failure) { error.value = failure } }
onMounted(async () => { await loadSources(); try { operation.value = await getOperations() } catch (failure) { error.value = failure } })
</script>

<style scoped>
.admin-sources { max-width: 1100px; margin: auto; padding: 24px; }.eyebrow { color: var(--zg-action); }section { margin: 24px 0; padding: 18px; border: 1px solid #ddd; }textarea { width: 100%; min-height: 180px; }.error { color: #b42318; }
</style>
