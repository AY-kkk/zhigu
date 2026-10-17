import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { parse, compileScript, compileTemplate } from '@vue/compiler-sfc'

const read = path => readFileSync(new URL(`../../src/${path}`, import.meta.url), 'utf8')

for (const file of [
  'view/research/new.vue',
  'view/research/detail.vue',
  'view/strategies/index.vue',
  'view/strategies/market.vue',
  'view/strategies/marketDetail.vue',
  'view/intel/index.vue',
]) {
  test(`existing module SFC remains compilable: ${file}`, () => {
    const { descriptor, errors } = parse(read(file), { filename: file })
    assert.deepEqual(errors, [])
    const script = descriptor.scriptSetup ? compileScript(descriptor, { id: file }) : null
    const template = compileTemplate({
      source: descriptor.template.content,
      filename: file,
      id: file,
      compilerOptions: { bindingMetadata: script?.bindings },
    })
    assert.deepEqual(template.errors, [])
  })
}

test('existing modules keep facts, provenance, stale and safe-link language', () => {
  const report = read('components/research/Report.vue')
  const strategy = read('components/strategies/RuleEditor.vue')
  const safeLink = read('components/research/SafeSourceLink.vue')
  assert.match(report, /font-size:\s*16px/)
  assert.match(report, /line-height:\s*28px/)
  assert.match(strategy, /历史模拟不代表真实持仓/)
  assert.match(safeLink, /noopener/)
  assert.match(safeLink, /noreferrer/)
})

test('source tree contains no persisted credential storage', () => {
  const files = [
    'utils/http.js', 'utils/intelHttp.js', 'stores/session.js',
    'stores/intelWorkspace.js', 'stores/strategyMarket.js', 'router/finance.js',
  ]
  for (const file of files) {
    const source = read(file)
    assert.doesNotMatch(source, /localStorage\.(?:get|set|remove)Item\(\s*['"`]zhigu_(?:token|role|user)/, file)
    assert.doesNotMatch(source, /sessionStorage\.(?:get|set|remove)Item\(\s*['"`]zhigu_(?:token|role|user)/, file)
  }
})
