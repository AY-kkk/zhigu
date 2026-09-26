<template>
  <div class="login-page zhigu-consumer">
    <div class="login">
    <ZhiguLogo variant="stacked" />
    <h1>知股受邀登录</h1>
    <el-form @submit.prevent="onSubmit">
      <el-input v-model="username" placeholder="用户名" />
      <el-input v-model="password" type="password" placeholder="密码" show-password />
      <el-button type="primary" native-type="submit">登录</el-button>
      <p v-if="error" class="err">{{ error }}</p>
    </el-form>
    </div>
  </div>
</template>
<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { safeInternalRedirect } from '../../router/finance.js'
import { login } from '../../api/research.js'
import { useSession } from '../../stores/session.js'
import ZhiguLogo from '../../components/brand/ZhiguLogo.vue'

const username = ref('')
const password = ref('')
const error = ref('')
const router = useRouter()
const route = useRoute()
const session = useSession()
async function onSubmit() {
  error.value = ''
  try {
    const res = await login(username.value, password.value)
    session.setAuth(res.data)
    router.push(safeInternalRedirect(route.query.redirect))
  } catch (e) {
    error.value = e.message || '登录失败'
  }
}
</script>
<style scoped>
.login-page {
  min-height: 100dvh;
  background: var(--zg-paper, #F7F5F0);
  padding: 24px;
}
.login {
  max-width: 360px;
  margin: 80px auto;
  background: var(--zg-surface, #FFFEFB);
  padding: 24px;
  border-radius: 10px;
  display: grid;
  gap: 12px;
  border: 1px solid var(--zg-line, #DEDBD4);
}
h1 { margin: 0; font-size: 22px; }
.err { color: var(--zg-error-fg, #A62F35); }
.login :deep(.el-button--primary) {
  background: var(--zg-action);
  border-color: var(--zg-action);
  min-height: 40px;
}
.login :deep(.el-button--primary:hover),
.login :deep(.el-button--primary:focus) {
  background: var(--zg-action-hover);
  border-color: var(--zg-action-hover);
}
</style>
