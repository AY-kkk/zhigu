<template>
  <article class="intel-event-card" :class="{ 'is-selected': selected }">
    <button type="button" class="intel-event-main" @click="$emit('select', event)">
      <div class="intel-event-topline">
        <span class="intel-event-type">{{ typeLabel }}</span>
        <span class="intel-pill" :class="`is-${event.verification}`">{{ verificationLabel }}</span>
        <span class="intel-pill is-soft">{{ phaseLabel }}</span>
        <span class="intel-pill is-soft">{{ freshnessLabel }}</span>
      </div>
      <h3>{{ event.title }}</h3>
      <p class="intel-event-meta">
        <span v-for="subject in event.subjects || []" :key="subject.code" class="intel-subject">
          {{ subject.code }} · {{ subject.role === 'subject' ? '主体' : '交易对手' }}
        </span>
        <span>支持度：{{ supportLabel }}</span>
        <span v-if="event.open_conflict_count">未决冲突 {{ event.open_conflict_count }}</span>
      </p>
    </button>
  </article>
</template>
<script setup>
import { computed } from 'vue'
const props = defineProps({ event: { type: Object, required: true }, selected: Boolean })
defineEmits(['select'])
const typeLabel = computed(() => ({ acquisition: '收购', earnings_forecast: '业绩预告', regulatory_investigation: '监管立案' }[props.event.type] || '事件'))
const verificationLabel = computed(() => ({ unverified: '未核实', confirmed: '已确认', denied: '已否认', disputed: '有争议' }[props.event.verification] || '待评估'))
const phaseLabel = computed(() => ({ unknown: '阶段未知', proposed: '方案推进', agreed: '已签约', completed: '已完成', terminated: '已终止', forecast_issued: '已发预告', results_published: '已出结果', withdrawn: '已撤回', opened: '已立案', decision_issued: '已出决定', closed: '已结案' }[props.event.phase] || '阶段未知'))
const freshnessLabel = computed(() => ({ fresh: '信息新鲜', stale: '信息偏旧', expired: '信息过期', unknown: '新鲜度未知' }[props.event.freshness] || '待评估'))
const supportLabel = computed(() => ({ high: '高', medium: '中', low: '低', unknown: '待评估' }[props.event.support_level] || '待评估'))
</script>
<style scoped>
.intel-event-card { border: 1px solid var(--zg-border); border-radius: 16px; background: var(--zg-surface); transition: border-color .2s, box-shadow .2s; }
.intel-event-card:hover, .intel-event-card.is-selected { border-color: var(--zg-action); box-shadow: 0 10px 28px color-mix(in srgb, var(--zg-action) 12%, transparent); }
.intel-event-main { display: block; width: 100%; padding: 18px; text-align: left; background: transparent; border: 0; color: inherit; cursor: pointer; }
.intel-event-topline { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; margin-bottom: 12px; }
.intel-event-type { color: var(--zg-action); font-size: 12px; font-weight: 700; letter-spacing: .08em; }
.intel-event-main h3 { margin: 0 0 10px; font-size: 17px; line-height: 1.45; }
.intel-event-meta { display: flex; flex-wrap: wrap; gap: 8px 14px; margin: 0; color: var(--zg-muted); font-size: 12px; }
.intel-subject { color: var(--zg-ink); }
.intel-pill { display: inline-flex; align-items: center; min-height: 22px; padding: 2px 8px; border-radius: 999px; background: color-mix(in srgb, var(--zg-action) 12%, white); color: var(--zg-action); font-size: 11px; }
.intel-pill.is-soft { background: var(--zg-soft); color: var(--zg-muted); }
.intel-pill.is-confirmed { background: #e8f7ee; color: #18794e; }
.intel-pill.is-denied { background: #fff0ee; color: #b42318; }
.intel-pill.is-disputed { background: #fff7df; color: #9a6700; }
@media (max-width: 767px) { .intel-event-main { padding: 15px; } .intel-event-main h3 { font-size: 16px; } }
</style>
