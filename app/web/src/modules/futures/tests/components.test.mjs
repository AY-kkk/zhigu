import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { parse, compileScript, compileTemplate } from '@vue/compiler-sfc'

for (const file of [
  'layout/FuturesLayout.vue', 'components/FuturesNav.vue',
  'pages/OverviewPage.vue', 'pages/ProductPage.vue', 'pages/NewResearchPage.vue',
  'pages/ResearchDetailPage.vue', 'pages/HypothesisPage.vue', 'pages/NotificationsPage.vue',
  'pages/AdminSourcesPage.vue'
]) {
  test(`isolated SFC compiles: ${file}`, () => {
    const source = readFileSync(new URL(`../${file}`, import.meta.url), 'utf8')
    const { descriptor, errors } = parse(source)
    assert.deepEqual(errors, [])
    const script = descriptor.scriptSetup ? compileScript(descriptor, { id: file }) : null
    const template = compileTemplate({
      source: descriptor.template.content, filename: file, id: file,
      compilerOptions: { bindingMetadata: script?.bindings }
    })
    assert.deepEqual(template.errors, [])
  })
}
