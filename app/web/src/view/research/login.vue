<template>
  <div class="login">
    <h1>知股受邀登录</h1>
    <el-form @submit.prevent="onSubmit">
      <el-input v-model="username" placeholder="用户名" />
      <el-input v-model="password" type="password" placeholder="密码" show-password />
      <el-button type="primary" native-type="submit">登录</el-button>
      <p v-if="error" class="err">{{ error }}</p>
    </el-form>
  </div>
</template>
<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { login } from '../../api/research.js'
import { useSession } from '../../stores/session.js'
const username = ref('invitee')
const password = ref('Passw0rd!')
const error = ref('')
const router = useRouter()
const session = useSession()
async function onSubmit() {
  error.value = ''
  try {
    const res = await login(username.value, password.value)
    session.setAuth(res.data)
    router.push('/app/research/new')
  } catch (e) {
    error.value = e.message || '登录失败'
  }
}
</script>
<style scoped>
.login { max-width: 360px; margin: 80px auto; background: #fff; padding: 24px; border-radius: 12px; display: grid; gap: 12px; }
.err { color: #c45656; }
</style>
