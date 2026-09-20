<template>
  <div class="shell">
    <header>
      <strong>知股管理</strong>
      <nav>
        <router-link to="/admin/ai-settings">模型与数据源</router-link>
        <router-link to="/admin/research-runs">运行记录</router-link>
        <router-link to="/app/research/new">返回客户</router-link>
      </nav>
    </header>
    <FixtureBanner />
    <main><router-view /></main>
  </div>
</template>
<script setup>
import FixtureBanner from '../../components/research/FixtureBanner.vue'
</script>
<style scoped>
.shell { max-width: 960px; margin: 0 auto; padding: 16px; }
header { display: flex; gap: 16px; flex-wrap: wrap; align-items: center; }
nav { display: flex; gap: 12px; flex-wrap: wrap; }
main { margin-top: 16px; background: #fff; padding: 16px; border-radius: 12px; }
</style>
