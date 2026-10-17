import { test } from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import { resolve } from 'node:path'

const root = resolve(import.meta.dirname, '../..')
const dist = resolve(root, 'dist')
const manifest = JSON.parse(readFileSync(resolve(dist, '.vite/manifest.json'), 'utf8'))
const gzipSize = file => gzipSync(readFileSync(resolve(dist, file)), { level: 9 }).length

test('initial JS, route chunks, and CSS stay within the product budget', () => {
  const entries = Object.values(manifest).filter(item => item.isEntry)
  assert.equal(entries.length, 1)
  const entry = entries[0]
  assert.ok(gzipSize(entry.file) <= 350 * 1024, `initial JS ${gzipSize(entry.file)}`)
  for (const item of Object.values(manifest)) {
    if (!item.file.endsWith('.js') || item.isEntry) continue
    assert.ok(gzipSize(item.file) <= 180 * 1024, `${item.file} ${gzipSize(item.file)}`)
  }
  for (const file of entry.css || []) {
    assert.ok(gzipSize(file) <= 100 * 1024, `${file} ${gzipSize(file)}`)
  }
})

test('production output has no public sourcemaps or credential-like values', () => {
  for (const value of Object.values(manifest)) {
    assert.equal(existsSync(resolve(dist, `${value.file}.map`)), false, value.file)
  }
  const bundled = Object.values(manifest).map(item => readFileSync(resolve(dist, item.file), 'utf8')).join('\n')
  for (const secret of ['BEGIN PRIVATE KEY', '__Host-zhigu_session', 'zhigu_token']) {
    assert.equal(bundled.includes(secret), false, secret)
  }
  assert.doesNotMatch(bundled, /Bearer\s+(?:eyJ|[A-Za-z0-9_-]{30,})/)
})
