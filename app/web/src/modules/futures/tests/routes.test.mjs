import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildFuturesRoutes } from '../routes.js'

test('disabled registration is empty; enabled registration is lazy and isolated', () => {
  assert.deepEqual(buildFuturesRoutes(false), [])
  const [route] = buildFuturesRoutes(true)
  assert.equal(route.path, '/app/futures')
  assert.equal(route.meta.module, 'futures')
  assert.equal(typeof route.component, 'function')
  assert.deepEqual(route.children.map(item => item.path), [
    '', 'products/:productId', 'research/new', 'research/:runId',
    'hypotheses/:id', 'notifications'
  ])
  for (const child of route.children) assert.equal(typeof child.component, 'function')
  assert.equal(route.children[1].meta.tab, 'product')
  assert.equal(route.children[5].meta.module, 'futures')
})
