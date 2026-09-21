<template>
  <div class="zhigu-page">
    <h1>模型与数据源</h1>
    <p>密钥只写不回显。保存后可做格式校验。阶段 A 的「测试」只检查协议/密钥是否填写/SSRF，<strong>不等于上游连接成功</strong>。真实模型能力测试属于阶段 B。</p>
    <el-form label-width="120px">
      <el-form-item label="Base URL"><el-input v-model="baseUrl" /></el-form-item>
      <el-form-item label="Model"><el-input v-model="model" /></el-form-item>
      <el-form-item label="API Key"><el-input v-model="apiKey" type="password" show-password /></el-form-item>
      <el-button type="primary" @click="save">保存</el-button>
      <el-button :disabled="!saved" @click="testCfg">测试</el-button>
      <el-button :disabled="!tested" title="阶段 A 仅格式校验，启用需阶段 B 连接验证" @click="activate">启用</el-button>
    </el-form>
    <p v-if="saved">已保存，has_key={{ saved.has_key }}，digest={{ saved.config_digest }}，status={{ saved.test_status }}</p>
    <p v-if="testResult">校验结果：{{ testResult.status }}。{{ testResult.note || '仅格式校验，未验证连接。' }}</p>
    <p v-if="error" class="err">{{ error }}</p>
    <h2>已保存配置</h2>
    <ul>
      <li v-for="row in configs" :key="row.id">{{ row.id }} · {{ row.kind }} · {{ row.test_status }} · {{ row.config_digest }}</li>
    </ul>
    <h2>数据源</h2>
    <el-form label-width="120px">
      <el-form-item label="名称"><el-input v-model="sourceName" /></el-form-item>
      <el-form-item label="Key"><el-input v-model="sourceKey" type="password" /></el-form-item>
      <el-button @click="saveSource">保存数据源</el-button>
    </el-form>
  </div>
</template>
<script setup>
import { onMounted, ref } from 'vue'
import http from '../../utils/http.js'
const baseUrl = ref('https://api.openai.com/v1')
const model = ref('gpt-4.1-mini')
const apiKey = ref('')
const saved = ref(null)
const tested = ref(false)
const testResult = ref(null)
const error = ref('')
const sourceName = ref('fixture')
const sourceKey = ref('fixture-placeholder')
const configs = ref([])
async function refresh() {
  try {
    const [models, sources] = await Promise.all([
      http.get('/api/finance/admin/model-configs', { params: { kind: 'model' } }),
      http.get('/api/finance/admin/data-sources')
    ])
    configs.value = [...(models.data.items || []), ...(sources.data.items || [])]
  } catch (e) {
    error.value = e.message || '无法加载配置列表'
  }
}
onMounted(refresh)
async function save() {
  error.value = ''
  tested.value = false
  try {
    const res = await http.post('/api/finance/admin/model-configs', {
      public_config: { base_url: baseUrl.value, model: model.value, protocol: 'openai-chat-completions' },
      api_key: apiKey.value
    })
    saved.value = res.data
    apiKey.value = ''
    await refresh()
  } catch (e) {
    error.value = e.message || '保存失败，请检查地址和密钥'
  }
}
async function testCfg() {
  error.value = ''
  try {
    const res = await http.post(`/api/finance/admin/model-config-versions/${saved.value.id}/test`, {})
    testResult.value = res.data
    tested.value = res.data.status === 'passed'
  } catch (e) {
    error.value = e.message || '测试失败'
  }
}
async function activate() {
  error.value = ''
  try {
    await http.post(`/api/finance/admin/model-config-versions/${saved.value.id}/activate`, {})
    saved.value = { ...saved.value, test_status: 'activated' }
    await refresh()
  } catch (e) {
    error.value = e.message || '启用失败'
  }
}
async function saveSource() {
  error.value = ''
  try {
    await http.post('/api/finance/admin/data-sources', {
      public_config: { name: sourceName.value, protocol: 'fixture' },
      api_key: sourceKey.value
    })
    sourceKey.value = ''
    await refresh()
  } catch (e) {
    error.value = e.message || '数据源保存失败'
  }
}
</script>
<style scoped>
.err { color: var(--zg-error-fg); background: var(--zg-error-bg); padding: 8px 12px; border-radius: var(--zg-radius-control); }
li { overflow-wrap: anywhere; color: var(--zg-text-secondary); font-family: var(--zg-font-number); font-size: 13px; line-height: 22px; }
</style>
