<template>
  <div class="zhigu-page">
    <Empty :image="h(IconInbox, { size: 'extra-large' })" title="交易策略" description="策略研究功能正在规划中。本期不提供收益曲线、回测结果、持仓或下单。">
      <Button type="primary" theme="solid" @click="goResearch">前往投研观点</Button>
    </Empty>
  </div>
</template>
<script setup>
import { h } from 'vue'
import { useRouter } from 'vue-router'
import { Button, Empty } from '@kousum/semi-ui-vue'
import { IconInbox } from '@kousum/semi-icons-vue'
const router = useRouter()
function goResearch() { router.push('/app/research/new') }
</script>
