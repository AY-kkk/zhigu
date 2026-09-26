<template>
  <div class="zhigu-page">
    <h1>个人界面</h1>
    <section class="account">
      <Avatar :alt="session.username" size="medium">{{ initial }}</Avatar>
      <div>
        <h2>{{ session.username || '受邀用户' }}</h2>
        <p class="meta">当前登录账户。不提供风险测评、资产绑定或付费中心。</p>
        <button type="button" class="zg-btn zg-btn-secondary" @click="onLogout">退出登录</button>
      </div>
    </section>
    <section>
      <h2>我的研究</h2>
      <p class="meta">回看观点与证据，请打开研究记录。</p>
      <button type="button" class="zg-btn zg-btn-primary" @click="router.push('/app/history')">查看我的研究</button>
    </section>
  </div>
</template>
<script setup>
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { Avatar } from '@kousum/semi-ui-vue'
import { useSession } from '../../stores/session.js'
import { useResearchConversation } from '../../stores/researchConversation.js'

const router = useRouter()
const session = useSession()
const conversation = useResearchConversation()
const initial = computed(() => (session.username || '知').slice(0, 1))
function onLogout() {
  conversation.resetAll()
  session.logout()
  router.push('/login')
}
</script>
<style scoped>
h1 { margin: 0 0 24px; font-size: 28px; line-height: 38px; font-weight: 600; }
.account { display: flex; gap: 16px; align-items: flex-start; margin-bottom: 32px; }
h2 { margin: 0 0 8px; font-size: 22px; line-height: 32px; }
.meta { color: var(--zg-text-secondary); margin: 8px 0 16px; }
</style>
