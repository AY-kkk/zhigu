<template>
  <div>
    <el-input v-model="text" type="textarea" :rows="8" maxlength="2000" show-word-limit placeholder="粘贴 20–2000 字的投资观点" />
    <p v-if="count < 20" class="hint">至少 20 字</p>
  </div>
</template>
<script setup>
import { computed } from 'vue'
const text = defineModel({ type: String, default: '' })
const count = computed(() => Array.from(text.value || '').length)
</script>
<style scoped>
.hint { color: #c45656; }
</style>
