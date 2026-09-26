<template>
  <section class="prov" aria-label="来源与使用权限">
    <h3>来源与使用权限</h3>
    <p v-if="rightsNote" class="rights">{{ rightsNote }}</p>
    <ul>
      <li v-for="(s, i) in entries" :key="i">
        <strong>{{ s.title }}</strong>
        <a
          v-if="safeUrl(s.url)"
          :href="s.url"
          target="_blank"
          rel="noopener noreferrer nofollow"
          class="link"
        >{{ s.url }}</a>
        <span v-else-if="s.record_ref" class="ref">内部记录号：{{ s.record_ref }}</span>
        <span v-else class="ref">（无链接）</span>
        <span class="meta">整理日期：{{ s.collected_at }}</span>
        <span class="meta">{{ s.adaptation }}</span>
      </li>
    </ul>
    <p v-if="!entries.length" class="meta">暂无来源记录。</p>
  </section>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  sources: { type: [Array, Object], default: () => [] },
  rightsNote: { type: String, default: '' }
})

const entries = computed(() => (Array.isArray(props.sources) ? props.sources : []))

// 来源 URL 只允许 http/https；内部来源编号只作纯文本，不拼成任意外链。
function safeUrl(url) {
  return typeof url === 'string' && (url.startsWith('https://') || url.startsWith('http://'))
}
</script>

<style scoped>
.prov {
  border: 1px solid var(--zg-line, #e5e7eb);
  border-radius: 10px;
  padding: 12px 16px;
}
h3 {
  margin: 0 0 8px;
  font-size: 15px;
}
.rights {
  margin: 0 0 8px;
  font-size: 13px;
  color: var(--zg-muted, #6b7280);
}
ul {
  margin: 0;
  padding-left: 18px;
}
li {
  margin-bottom: 6px;
  font-size: 13px;
}
.meta {
  margin-left: 8px;
  color: var(--zg-muted, #6b7280);
}
.link {
  margin-left: 8px;
  word-break: break-all;
}
.ref {
  margin-left: 8px;
  color: var(--zg-muted, #6b7280);
}
</style>
