<template>
  <div class="search" role="search">
    <div class="seg" role="group" aria-label="市场">
      <button type="button" class="zg-btn zg-btn-ghost" :class="{ on: store.searchMarket === '' }" @click="setMarket('')">全部</button>
      <button type="button" class="zg-btn zg-btn-ghost" :class="{ on: store.searchMarket === 'A' }" @click="setMarket('A')">A股</button>
      <button type="button" class="zg-btn zg-btn-ghost" :class="{ on: store.searchMarket === 'HK' }" @click="setMarket('HK')">港股</button>
    </div>
    <label class="sr" for="zg-inst-q">搜索股票</label>
    <input
      id="zg-inst-q"
      v-model="store.query"
      class="q"
      placeholder="代码、名称、拼音，如 700 / 茅台 / gzmt；空搜索可浏览全部"
      autocomplete="off"
      @focus="onFocus"
      @input="onInput"
      @keydown.down.prevent="move(1)"
      @keydown.up.prevent="move(-1)"
      @keydown.enter.prevent="pickActive"
    >
    <p v-if="store.searchError" class="err">{{ store.searchError }}</p>
    <ul v-if="store.hits.length" class="hits" role="listbox" @scroll="onScroll">
      <li
        v-for="(row, i) in store.hits"
        :key="row.instrument_id"
        :class="{ on: i === active }"
        role="option"
        :aria-selected="i === active"
        @mousedown.prevent="pick(row)"
        @click.prevent="pick(row)"
      >
        <strong>{{ row.name }}</strong>
        <span>{{ row.instrument_id }}</span>
        <b v-if="row.last" data-testid="hit-last" :class="tone(row)">{{ row.last }}<template v-if="row.change_pct"> {{ fmtPct(row.change_pct) }}</template></b>
        <em>{{ row.exchange }} · {{ row.currency }} · {{ row.asset_type }}</em>
        <small v-if="row.unsupported_reason">{{ row.unsupported_reason }}</small>
      </li>
      <li v-if="store.searchCursor" class="more" role="presentation">
        <button type="button" class="zg-btn zg-btn-ghost" :disabled="store.searchBusy" @mousedown.prevent="store.search(true)">{{ store.searchBusy ? '加载中' : '更多股票' }}</button>
      </li>
    </ul>
  </div>
</template>
<script setup>
import { ref } from 'vue'
import { useStrategyWorkspace } from '../../stores/strategyWorkspace.js'

const store = useStrategyWorkspace()
const active = ref(0)
let timer
function onFocus() {
  if (!store.hits.length) store.search()
}
function onInput() {
  clearTimeout(timer)
  timer = setTimeout(() => store.search(), 300)
}
function setMarket(m) {
  store.setSearchMarket(m)
}
function onScroll(ev) {
  const el = ev.target
  if (!store.searchCursor || store.searchBusy) return
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 24) store.search(true)
}
function move(d) {
  if (!store.hits.length) return
  active.value = (active.value + d + store.hits.length) % store.hits.length
}
function pick(row) {
  store.query = `${row.name} ${row.instrument_id}`
  store.hits = []
  store.searchCursor = null
  store.selectInstrument(row.instrument_id)
}
function pickActive() {
  if (store.hits[active.value]) pick(store.hits[active.value])
}
function tone(row) {
  return Number(row.change) >= 0 ? 'up' : 'down'
}
function fmtPct(v) {
  const n = Number(v)
  if (!Number.isFinite(n)) return ''
  return `${n > 0 ? '+' : ''}${n}%`
}
</script>
<style scoped>
.search { position: relative; min-width: 0; flex: 1; display: flex; align-items: center; gap: 8px; max-width: 560px; }
.seg { display: flex; gap: 4px; flex: none; }
.seg .on, .seg button.on { background: var(--zg-surface-muted); }
.q {
  flex: 1; min-width: 0; width: auto; height: 36px; padding: 0 10px;
  border: 1px solid var(--zg-control-border); border-radius: 6px; background: var(--zg-surface); font: inherit;
}
.hits {
  position: absolute; z-index: 20; left: 0; right: 0; margin: 4px 0 0; padding: 4px 0;
  list-style: none; background: var(--zg-surface); border: 1px solid var(--zg-line); max-height: 360px; overflow: auto;
}
.hits li { display: grid; gap: 2px; padding: 8px 12px; cursor: pointer; }
.hits li.on, .hits li:hover { background: var(--zg-surface-muted); }
.hits li.more { cursor: default; padding: 4px 8px; }
.hits span, .hits em, .hits small { font-size: 12px; color: var(--zg-text-secondary); }
.hits b { font-size: 12px; font-weight: 600; }
.hits b.up { color: var(--zg-up, #c62828); }
.hits b.down { color: var(--zg-down, #2e7d32); }
.err { color: var(--zg-error-fg); font-size: 12px; margin: 4px 0 0; }
.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
</style>
