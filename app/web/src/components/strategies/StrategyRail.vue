<template>
  <aside class="side" :class="{ drawer, fill: mobile }">
    <section class="block">
      <h2>策略助手</h2>
      <p class="hint">用一句话描述买卖规则。未写明指标时，按 MACD 金叉开仓、死叉平仓。收益是历史模拟，不是实盘。</p>
      <label class="saved">已保存策略
        <select aria-label="已保存策略" :value="store.strategyId" @change="store.openStrategy($event.target.value)">
          <option value="">新策略</option>
          <option v-for="s in store.strategies" :key="s.strategy_id" :value="s.strategy_id">{{ s.name }}</option>
        </select>
      </label>
      <textarea v-model="store.prompt" maxlength="4000" rows="4" placeholder="例如：MACD 金叉且 KDJ 的 K 小于 30 时半仓买入，MACD 死叉卖出；也可只写「金叉买入」。"></textarea>
      <div class="actions">
        <button type="button" class="zg-btn zg-btn-primary" data-testid="generate-rule" :aria-busy="busyGen" @click="store.generate()">生成规则</button>
        <button v-if="busyGen" type="button" class="zg-btn zg-btn-ghost" @click="store.cancelGenerate()">取消生成</button>
      </div>
      <p data-testid="gen-status" class="meta">状态 {{ store.genStatus || 'idle' }}</p>
      <p v-if="store.genError" class="err">{{ store.genError }}</p>
    </section>

    <section v-if="questions.length" class="block">
      <h3>需要澄清</h3>
      <ul class="plain"><li v-for="q in questions" :key="q">{{ q }}</li></ul>
    </section>

    <section class="block" data-testid="rule-card" :key="store.draftRev">
      <template v-if="store.draft?.dsl">
        <h3>{{ store.draft.dsl.name || '规则' }}</h3>
        <p class="meta">{{ store.draft.dsl.instrument_id }} · {{ store.draft.dsl.signal_period }} · {{ store.draft.dsl.price_basis }}</p>
        <p data-testid="position-value">仓位 {{ store.draft.dsl.position?.value }} · 止损 {{ store.draft.dsl.risk?.stop_loss_pct || '未启用' }}</p>
        <p class="meta">执行 {{ store.draft.dsl.execution?.timing }}，次一交易日开盘</p>
        <label v-if="store.kdjK !== ''">KDJ K 阈值
          <input :value="store.kdjK" type="number" min="0" max="100" @change="store.setKdjK($event.target.value)">
        </label>
        <ul v-if="assumptions.length" class="plain">
          <li v-for="a in assumptions" :key="a">{{ a }}</li>
        </ul>
        <div class="actions">
          <button type="button" class="zg-btn zg-btn-ghost" @click="store.applyDraft()">应用修改</button>
          <button type="button" class="zg-btn zg-btn-ghost" :disabled="store.saveBusy" @click="store.saveCurrent()">保存策略</button>
          <button type="button" class="zg-btn zg-btn-ghost" @click="store.showAdvanced = !store.showAdvanced">{{ store.showAdvanced ? '隐藏 DSL' : '查看 DSL' }}</button>
        </div>
        <p v-if="store.saveNotice" class="meta">{{ store.saveNotice }}</p>
        <p v-if="store.saveError" class="err">{{ store.saveError }}</p>
        <textarea v-if="store.showAdvanced" v-model="store.dslText" class="dsl" rows="10" aria-label="策略 DSL"></textarea>
      </template>
      <p v-else class="hint">生成后在这里改入场、退出和仓位。</p>
    </section>

    <section class="block">
      <h3>回测</h3>
      <p class="hint">按初始资金和起止日期，在当前这一只股票上找出买点和卖点。</p>
      <div class="dates">
        <label>起始 <input v-model="store.start" type="date"></label>
        <label>结束 <input v-model="store.end" type="date"></label>
      </div>
      <label>初始资金 <input v-model="store.initialCash" inputmode="decimal"></label>
      <details>
        <summary>费用假设</summary>
        <label>滑点 bps <input v-model="store.slippage"></label>
        <p class="hint">佣金按产品默认 2.5bp 估计，不是券商费率。</p>
      </details>
      <div class="actions">
        <button type="button" class="zg-btn zg-btn-primary" data-testid="run-backtest" :disabled="store.btBusy || !store.draft?.dsl" @click="store.runBacktest()">运行回测</button>
        <button v-if="store.btBusy" type="button" class="zg-btn zg-btn-ghost" @click="store.cancelBacktest()">取消回测</button>
      </div>
    </section>
  </aside>
</template>
<script setup>
import { computed } from 'vue'
import { useStrategyWorkspace } from '../../stores/strategyWorkspace.js'

defineProps({
  drawer: { type: Boolean, default: false },
  mobile: { type: Boolean, default: false }
})

const store = useStrategyWorkspace()
const busyGen = computed(() => store.genStatus === 'generating' || store.genStatus === 'queued')
const questions = computed(() => (Array.isArray(store.draft?.clarification) ? store.draft.clarification : []))
const assumptions = computed(() => (Array.isArray(store.draft?.assumptions) ? store.draft.assumptions : []))
</script>
<style scoped>
.side {
  width: 320px; flex: none; overflow: auto; padding: 12px 14px 20px; position: relative; z-index: 2;
  border-left: 1px solid var(--zg-line); background: var(--zg-surface);
}
.side.drawer { position: absolute; right: 0; top: 0; bottom: 0; z-index: 15; box-shadow: var(--zg-shadow-overlay); }
.side.fill { width: 100%; border-left: 0; }
.block { display: grid; gap: 8px; padding-top: 14px; margin-top: 14px; border-top: 1px solid var(--zg-line); }
.block:first-child { padding-top: 0; margin-top: 0; border-top: 0; }
h2, h3 { margin: 0; font-size: 16px; font-family: var(--zg-font-editorial); font-weight: 600; }
h3 { font-size: 15px; }
textarea, input, select {
  width: 100%; border: 1px solid var(--zg-control-border); border-radius: 6px; padding: 8px; font: inherit; background: #fff;
}
.hint, .meta { margin: 0; font-size: 12px; color: var(--zg-text-secondary); line-height: 1.45; }
.err { margin: 0; color: var(--zg-error-fg); font-size: 13px; }
.dsl { font-family: var(--zg-font-number); font-size: 11px; }
.actions { display: flex; flex-wrap: wrap; gap: 8px; }
.actions .zg-btn { min-height: 36px; padding: 0 12px; }
.saved, label { display: grid; gap: 4px; font-size: 12px; }
.dates { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
.plain { margin: 0; padding-left: 18px; font-size: 12px; color: var(--zg-text-secondary); }
details summary { cursor: pointer; font-size: 12px; color: var(--zg-text-secondary); }
details label { margin-top: 8px; }
</style>
