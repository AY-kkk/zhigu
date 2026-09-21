<template>
  <div class="search" role="search">
    <label class="sr" for="zg-inst-q">搜索股票</label>
    <input
      id="zg-inst-q"
      v-model="store.query"
      class="q"
      placeholder="代码、名称、拼音，如 700 / 茅台 / gzmt"
      autocomplete="off"
      @input="onInput"
      @keydown.down.prevent="move(1)"
      @keydown.up.prevent="move(-1)"
      @keydown.enter.prevent="pickActive"
    >
    <p v-if="store.searchError" class="err">{{ store.searchError }}</p>
    <ul v-if="store.hits.length" class="hits" role="listbox">
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
        <em>{{ row.exchange }} · {{ row.currency }} · {{ row.asset_type }}</em>
        <small v-if="row.unsupported_reason">{{ row.unsupported_reason }}</small>
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
function onInput() {
  clearTimeout(timer)
  timer = setTimeout(() => store.search(), 300)
}
function move(d) {
  if (!store.hits.length) return
  active.value = (active.value + d + store.hits.length) % store.hits.length
}
function pick(row) {
  store.query = `${row.name} ${row.instrument_id}`
  store.hits = []
  store.selectInstrument(row.instrument_id)
}
function pickActive() {
  if (store.hits[active.value]) pick(store.hits[active.value])
}
</script>
<style scoped>
.search { position: relative; min-width: 240px; flex: 1; }
.q {
  width: 100%; height: 36px; padding: 0 10px;
  border: 1px solid var(--zg-control-border); border-radius: 6px; background: var(--zg-surface); font: inherit;
}
.hits {
  position: absolute; z-index: 20; left: 0; right: 0; margin: 4px 0 0; padding: 4px 0;
  list-style: none; background: var(--zg-surface); border: 1px solid var(--zg-line); max-height: 280px; overflow: auto;
}
.hits li { display: grid; gap: 2px; padding: 8px 12px; cursor: pointer; }
.hits li.on, .hits li:hover { background: var(--zg-surface-muted); }
.hits span, .hits em, .hits small { font-size: 12px; color: var(--zg-text-secondary); }
.err { color: var(--zg-error-fg); font-size: 12px; margin: 4px 0 0; }
.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
</style>
