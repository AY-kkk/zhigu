<template>
  <section class="intel-replay">
    <div class="intel-replay-head">
      <div>
        <p class="eyebrow">隔离回放</p>
        <h2>演示数据时间线</h2>
      </div>
      <span class="intel-watermark">演示数据</span>
    </div>
    <p class="intel-replay-note">下一步只写入当前演示空间，不写入真实数据或真实通知。重置会清除该空间业务状态并递增 generation。</p>
    <div class="intel-replay-actions">
      <button class="zg-btn zg-btn-primary" :disabled="busy" @click="$emit('step')">下一步</button>
      <button class="zg-btn zg-btn-ghost" :disabled="busy" @click="$emit('reset')">重置</button>
    </div>
    <p v-if="error" class="intel-error">{{ error }}</p>
  </section>
</template>
<script setup>
defineProps({ busy: Boolean, error: String })
defineEmits(['step', 'reset'])
</script>
<style scoped>
.intel-replay { padding: 20px; border-radius: 18px; background: linear-gradient(135deg, #fff8f7, #fff); border: 1px solid #f1d6d1; }
.intel-replay-head { display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; }
.eyebrow { margin: 0 0 4px; color: var(--zg-action); font-size: 11px; font-weight: 700; letter-spacing: .12em; text-transform: uppercase; }
.intel-replay h2 { margin: 0; font-size: 20px; }
.intel-watermark { padding: 5px 9px; border: 1px solid #e7a69c; border-radius: 999px; color: #b5473b; font-size: 11px; font-weight: 700; }
.intel-replay-note { margin: 14px 0; color: var(--zg-muted); font-size: 13px; line-height: 1.7; }
.intel-replay-actions { display: flex; gap: 8px; }
.intel-error { margin: 12px 0 0; color: #b42318; font-size: 13px; }
</style>
