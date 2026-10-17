# 知股前端产品化收尾 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the frontend productization contract in `spec/前端产品化_开发SPEC.md`, including the shared product shell, normalized state/error contracts, complete futures journeys, regression coverage, accessibility, responsive behavior, and release evidence.

**Architecture:** Keep Vue 3, Vite, Vue Router, Pinia, Semi UI Vue, Element Plus, and lightweight-charts. Pages consume normalized view models; typed module clients own API calls; stores own concurrency and route/account invalidation; common components render state without making requests. Backend contracts remain read-only inputs.

**Tech Stack:** Vue 3.5, Vite 6, Vue Router 4, Pinia 2, Semi UI Vue 2.78, Element Plus 2.9, Axios 1.8, Vitest 3, @vue/test-utils 2, jsdom 26, Playwright 1.50, axe-core 4, lightweight-charts 5.

## Global Constraints

- Four state dimensions stay independent: `deployment_environment`, `operation_mode`, `data_provenance`, `health`.
- `off/read_only/live` are operational states, not rollout levels; `fixture/real` is provenance.
- Browser auth uses HttpOnly cookies; session/JWT/bearer tokens must never enter Web Storage or bundle output.
- CSRF tokens are session-bound and memory-only.
- Every request consumes `{data,error,trace_id}`; GET retries are bounded and side effects are never automatically retried.
- Stable error `code`, `retryable`, optional `retry_after_seconds`, and trace ID must match `app/contracts/error-codes.json`.
- Unknown model/source HTML never enters the DOM unsanitized.
- Required viewports are 1440, 1024, 390, and 320 px with no page-level horizontal overflow.
- Report body text is at least 16 px with 1.5–1.75 line height.
- Normal gates cannot silently skip. Only real provider/model probes may report `NOT_RUN`.
- Existing research, strategy, intelligence, and admin behavior must not regress.
- Do not change backend request/response contracts in this plan.

## File Structure

- `app/web/src/utils/runtimeState.js`: validates and normalizes four-dimensional capability state.
- `app/web/src/utils/apiEnvelope.js`: validates envelopes, maps stable errors, and parses page cursors.
- `app/web/src/utils/requestRunner.js`: bounded GET retry, abort, sequencing, and side-effect safety.
- `app/web/src/components/system/*`: normalized status, freshness, provenance, error, empty, loading, mode, and confirm components.
- `app/web/src/components/workspace/*`: product shell, navigation, header, page heading, and route error boundary.
- `app/web/src/modules/futures/model.js`: pure futures view-model normalization.
- `app/web/src/modules/futures/store.js`: request state, cancellation, polling, invalidation, and idempotency keys.
- `app/web/src/modules/futures/pages/*`: overview, product, research creation/detail, hypotheses, notifications, and source administration.
- `app/web/src/api/futures.js`: typed API calls with strict query/body mapping.
- `app/web/src/router/finance.js`: route metadata, focus behavior, and recoverable 404.
- `app/web/tests/*`: Vitest unit/component tests.
- `app/web/e2e/*`: Playwright journeys, accessibility, responsive, storage, and failure tests.

---

## Task 1: Freeze frontend contracts and quality tooling

**Files:**
- Modify: `app/web/package.json`
- Create: `app/web/vitest.config.js`
- Create: `app/web/src/utils/runtimeState.js`
- Create: `app/web/src/utils/apiEnvelope.js`
- Create: `app/web/src/utils/requestRunner.js`
- Test: `app/web/tests/unit/runtimeState.test.js`
- Test: `app/web/tests/unit/apiEnvelope.test.js`
- Test: `app/web/tests/unit/requestRunner.test.js`

**Interfaces:**
- Consumes: backend capability fields and error envelopes as plain objects.
- Produces: `normalizeRuntimeState(input)`, `unwrapEnvelope(value)`, `normalizePage(value)`, `runRequest(options)`, `createRequestSequence()`.

- [ ] **Step 1: Add deterministic test tooling**

Replace the `scripts` and `devDependencies` portions of `app/web/package.json` with:

```json
{
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview",
    "test": "vitest run",
    "test:watch": "vitest",
    "test:e2e": "playwright test",
    "test:axe": "playwright test e2e/accessibility.spec.js",
    "check:web": "npm run test && npm run build"
  },
  "devDependencies": {
    "@playwright/test": "1.50.0",
    "@vitejs/plugin-vue": "5.2.1",
    "@vue/test-utils": "2.4.6",
    "axe-core": "4.10.3",
    "jsdom": "26.0.0",
    "vite": "6.0.7",
    "vitest": "3.0.8"
  }
}
```

Create `app/web/vitest.config.js`:

```js
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  test: {
    environment: 'jsdom',
    include: ['tests/**/*.test.js'],
    restoreMocks: true,
    clearMocks: true,
  },
})
```

Run: `npm install && npm test`
Expected: Vitest exits 1 because no test files are included yet.

- [ ] **Step 2: Write runtime-state tests**

Create `app/web/tests/unit/runtimeState.test.js`:

```js
import { describe, expect, it } from 'vitest'
import { normalizeRuntimeState } from '../../src/utils/runtimeState'

describe('normalizeRuntimeState', () => {
  it('keeps four dimensions independent', () => {
    expect(normalizeRuntimeState({
      deployment_environment: 'demo',
      operation_mode: 'read_only',
      data_provenance: 'fixture',
      health: 'degraded',
      reason: 'MODEL_UNAVAILABLE',
    })).toEqual({
      deploymentEnvironment: 'demo',
      operationMode: 'read_only',
      dataProvenance: 'fixture',
      health: 'degraded',
      reason: 'MODEL_UNAVAILABLE',
      blocking: true,
    })
  })

  it('fail-closes production fixture and illegal values', () => {
    const value = normalizeRuntimeState({
      deployment_environment: 'production',
      operation_mode: 'live',
      data_provenance: 'fixture',
      health: 'ready',
    })
    expect(value.dataProvenance).toBe('real')
    expect(value.operationMode).toBe('read_only')
    expect(value.health).toBe('unavailable')
    expect(value.reason).toBe('INVALID_DATA_PROVENANCE')
  })
})
```

Run: `npx vitest run tests/unit/runtimeState.test.js`
Expected: FAIL with unresolved `runtimeState.js`.

- [ ] **Step 3: Implement runtime-state normalization**

Create `app/web/src/utils/runtimeState.js`:

```js
const ENVIRONMENTS = new Set(['test', 'demo', 'production'])
const MODES = new Set(['off', 'read_only', 'live'])
const PROVENANCE = new Set(['fixture', 'real'])
const HEALTH = new Set(['ready', 'degraded', 'unavailable'])

export function normalizeRuntimeState(input = {}) {
  const value = {
    deploymentEnvironment: ENVIRONMENTS.has(input.deployment_environment) ? input.deployment_environment : 'test',
    operationMode: MODES.has(input.operation_mode) ? input.operation_mode : 'off',
    dataProvenance: PROVENANCE.has(input.data_provenance) ? input.data_provenance : 'fixture',
    health: HEALTH.has(input.health) ? input.health : 'unavailable',
    reason: String(input.reason || ''),
    blocking: true,
  }
  if (value.deploymentEnvironment === 'production' && value.dataProvenance === 'fixture') {
    value.dataProvenance = 'real'
    value.operationMode = 'read_only'
    value.health = 'unavailable'
    value.reason = 'INVALID_DATA_PROVENANCE'
  }
  value.blocking = value.operationMode !== 'live' || value.health !== 'ready'
  return value
}
```

Run: `npx vitest run tests/unit/runtimeState.test.js`
Expected: PASS.

- [ ] **Step 4: Write envelope and pagination tests**

Create `app/web/tests/unit/apiEnvelope.test.js`:

```js
import { describe, expect, it } from 'vitest'
import { normalizePage, unwrapEnvelope } from '../../src/utils/apiEnvelope'

describe('apiEnvelope', () => {
  it('preserves stable errors and trace IDs', () => {
    const value = unwrapEnvelope({
      data: null,
      error: { code: 'FUTURES_READ_ONLY', message: 'read only', retryable: false },
      trace_id: '550e8400-e29b-41d4-a716-446655440000',
    })
    expect(value.error.code).toBe('FUTURES_READ_ONLY')
    expect(value.traceId).toBe('550e8400-e29b-41d4-a716-446655440000')
  })

  it('rejects malformed success envelopes and keeps pagination fields', () => {
    expect(() => unwrapEnvelope({ data: {}, error: {} })).toThrow()
    expect(normalizePage({ items: [], limit: 20, cursor: null, next_cursor: null })).toEqual({
      items: [], limit: 20, cursor: null, nextCursor: null,
    })
  })
})
```

Run: `npx vitest run tests/unit/apiEnvelope.test.js`
Expected: FAIL with unresolved `apiEnvelope.js`.

- [ ] **Step 5: Implement envelope and pagination normalization**

Create `app/web/src/utils/apiEnvelope.js`:

```js
export class ContractError extends Error {
  constructor(code, message, options = {}) {
    super(message || code)
    this.name = 'ContractError'
    this.code = code
    this.retryable = Boolean(options.retryable)
    this.retryAfterSeconds = Number.isFinite(options.retryAfterSeconds) ? options.retryAfterSeconds : null
    this.traceId = options.traceId || ''
  }
}

export function unwrapEnvelope(value) {
  if (!value || typeof value !== 'object' || !('data' in value) || !('error' in value) || typeof value.trace_id !== 'string') {
    throw new ContractError('CONTRACT_ENVELOPE_INVALID', '响应不符合 envelope 契约')
  }
  if (value.error !== null) {
    const error = value.error
    if (!error || typeof error.code !== 'string') throw new ContractError('CONTRACT_ENVELOPE_INVALID', '错误响应缺少稳定 code')
    throw new ContractError(error.code, error.message, {
      retryable: error.retryable,
      retryAfterSeconds: error.retry_after_seconds,
      traceId: value.trace_id,
    })
  }
  return { data: value.data, traceId: value.trace_id }
}

export function normalizePage(value) {
  if (!value || !Array.isArray(value.items) || !Number.isInteger(value.limit)) {
    throw new ContractError('CONTRACT_PAGE_INVALID', '分页响应不符合契约')
  }
  return {
    items: value.items,
    limit: value.limit,
    cursor: value.cursor ?? null,
    nextCursor: value.next_cursor ?? null,
  }
}
```

Run: `npx vitest run tests/unit/apiEnvelope.test.js`
Expected: PASS.

- [ ] **Step 6: Write request concurrency tests**

Create `app/web/tests/unit/requestRunner.test.js`:

```js
import { describe, expect, it, vi } from 'vitest'
import { createRequestSequence, runRequest } from '../../src/utils/requestRunner'

describe('requestRunner', () => {
  it('never retries side effects and bounds GET retries', async () => {
    const get = vi.fn().mockRejectedValue(new Error('network'))
    await expect(runRequest({ method: 'GET', request: get, retries: 2, delay: () => 0 })).rejects.toThrow('network')
    expect(get).toHaveBeenCalledTimes(3)
    const post = vi.fn().mockRejectedValue(new Error('network'))
    await expect(runRequest({ method: 'POST', request: post, retries: 5, delay: () => 0 })).rejects.toThrow('network')
    expect(post).toHaveBeenCalledTimes(1)
  })

  it('discards stale sequence results', async () => {
    const sequence = createRequestSequence()
    const first = sequence.begin()
    const second = sequence.begin()
    expect(sequence.isCurrent(first)).toBe(false)
    expect(sequence.isCurrent(second)).toBe(true)
  })
})
```

Run: `npx vitest run tests/unit/requestRunner.test.js`
Expected: FAIL with unresolved `requestRunner.js`.

- [ ] **Step 7: Implement safe request execution**

Create `app/web/src/utils/requestRunner.js`:

```js
export function createRequestSequence() {
  let current = 0
  return {
    begin() { current += 1; return current },
    isCurrent(id) { return id === current },
    invalidate() { current += 1 },
  }
}

export async function runRequest({ method = 'GET', request, retries = 0, delay = ms => new Promise(resolve => setTimeout(resolve, ms)) }) {
  const attempts = method.toUpperCase() === 'GET' ? Math.max(1, retries + 1) : 1
  let lastError
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      return await request()
    } catch (error) {
      lastError = error
      if (attempt + 1 < attempts) await delay(250 * (2 ** attempt))
    }
  }
  throw lastError
}
```

Run: `npx vitest run tests/unit/requestRunner.test.js`
Expected: PASS.

- [ ] **Step 8: Run the package gate and commit**

Run: `npm run check:web`
Expected: tests pass and production build exits 0.

```bash
git add package.json package-lock.json vitest.config.js tests/unit src/utils/runtimeState.js src/utils/apiEnvelope.js src/utils/requestRunner.js
git commit -m "feat(web): freeze runtime and request contracts"
```

---

## Task 2: Normalize system state components

**Files:**
- Create: `app/web/src/components/system/CapabilityStatus.vue`
- Create: `app/web/src/components/system/DataFreshness.vue`
- Create: `app/web/src/components/system/ProvenanceSummary.vue`
- Create: `app/web/src/components/system/RequestError.vue`
- Create: `app/web/src/components/system/EmptyState.vue`
- Create: `app/web/src/components/system/LoadingState.vue`
- Create: `app/web/src/components/system/ModeBanner.vue`
- Create: `app/web/src/components/system/ConfirmAction.vue`
- Test: `app/web/tests/components/systemState.test.js`
- Modify: `app/web/src/styles/editorial-tokens.css`

**Interfaces:**
- Consumes: normalized runtime state from Task 1 and page-owned display props.
- Produces: stable component prop contracts used by all later pages.

- [ ] **Step 1: Write component contract tests**

Create `app/web/tests/components/systemState.test.js`:

```js
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CapabilityStatus from '../../src/components/system/CapabilityStatus.vue'
import DataFreshness from '../../src/components/system/DataFreshness.vue'
import RequestError from '../../src/components/system/RequestError.vue'

describe('system state components', () => {
  it('shows all four dimensions and non-color status text', () => {
    const wrapper = mount(CapabilityStatus, {
      props: { state: {
        deploymentEnvironment: 'production', operationMode: 'read_only',
        dataProvenance: 'real', health: 'unavailable', reason: 'MODEL_UNAVAILABLE', blocking: true,
      } },
    })
    expect(wrapper.text()).toContain('production')
    expect(wrapper.text()).toContain('read_only')
    expect(wrapper.text()).toContain('real')
    expect(wrapper.text()).toContain('unavailable')
    expect(wrapper.text()).toContain('MODEL_UNAVAILABLE')
  })

  it('separates freshness from provenance', () => {
    const wrapper = mount(DataFreshness, {
      props: { asOf: '2026-10-13T10:00:00Z', updatedAt: '2026-10-13T10:05:00Z', stale: true, reason: 'UPSTREAM_DELAY' },
    })
    expect(wrapper.text()).toContain('数据截止')
    expect(wrapper.text()).toContain('更新')
    expect(wrapper.text()).toContain('陈旧')
    expect(wrapper.attributes('role')).toBe('status')
  })

  it('keeps recoverable errors on the page with trace ID', () => {
    const wrapper = mount(RequestError, {
      props: { error: { code: 'UPSTREAM_TIMEOUT', message: '请求超时', retryable: true, traceId: 'trace-1' } },
    })
    expect(wrapper.text()).toContain('UPSTREAM_TIMEOUT')
    expect(wrapper.text()).toContain('trace-1')
    expect(wrapper.find('button').exists()).toBe(true)
  })
})
```

Run: `npx vitest run tests/components/systemState.test.js`
Expected: FAIL because component files do not exist.

- [ ] **Step 2: Define shared status tokens**

Append to `app/web/src/styles/editorial-tokens.css`:

```css
.system-status {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2, 8px);
  min-height: 28px;
  padding: 4px 8px;
  border: 1px solid currentColor;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 650;
  line-height: 1.4;
}
.system-status[data-tone='ready'] { color: var(--color-success, #237a4b); }
.system-status[data-tone='warning'] { color: var(--color-warning, #9a5b00); }
.system-status[data-tone='blocked'] { color: var(--color-danger, #b4232c); }
.system-status[data-tone='neutral'] { color: var(--color-text-secondary, #5f5962); }
.system-error {
  padding: var(--space-4, 16px);
  border-inline-start: 4px solid var(--color-danger, #b4232c);
  background: var(--color-surface-raised, #fff);
}
```

- [ ] **Step 3: Implement normalized system components**

Each component accepts display-only props and emits user intent; none calls an API.

`CapabilityStatus.vue`:

```vue
<script setup>
import { computed } from 'vue'
const props = defineProps({ state: { type: Object, required: true } })
const rows = computed(() => [
  ['部署环境', props.state.deploymentEnvironment],
  ['操作模式', props.state.operationMode],
  ['数据来源', props.state.dataProvenance],
  ['健康状态', props.state.health],
])
const tone = computed(() => props.state.health === 'ready' ? 'ready' : props.state.health === 'degraded' ? 'warning' : 'blocked')
</script>
<template>
  <section class="capability-status" aria-label="模块运行状态">
    <span class="system-status" :data-tone="tone">{{ state.health }}</span>
    <dl>
      <div v-for="[label, value] in rows" :key="label"><dt>{{ label }}</dt><dd>{{ value }}</dd></div>
    </dl>
    <p v-if="state.reason">{{ state.reason }}</p>
  </section>
</template>
```

`DataFreshness.vue`:

```vue
<script setup>
defineProps({
  asOf: { type: String, default: '' },
  updatedAt: { type: String, default: '' },
  stale: { type: Boolean, default: false },
  reason: { type: String, default: '' },
})
</script>
<template>
  <p class="data-freshness" role="status">
    <span>数据截止：{{ asOf || '未提供' }}</span>
    <span>更新：{{ updatedAt || '未提供' }}</span>
    <span v-if="stale" class="system-status" data-tone="warning">陈旧{{ reason ? `：${reason}` : '' }}</span>
  </p>
</template>
```

`ProvenanceSummary.vue`:

```vue
<script setup>
defineProps({ source: { type: String, required: true }, version: { type: String, default: '' }, license: { type: String, default: '' } })
</script>
<template>
  <dl class="provenance-summary"><dt>来源</dt><dd>{{ source }}</dd><dt v-if="version">版本</dt><dd v-if="version">{{ version }}</dd><dt v-if="license">权限</dt><dd v-if="license">{{ license }}</dd></dl>
</template>
```

`RequestError.vue`:

```vue
<script setup>
defineProps({ error: { type: Object, required: true } })
defineEmits(['retry'])
</script>
<template>
  <section class="system-error" role="alert">
    <strong>{{ error.code }}</strong>
    <p>{{ error.message }}</p>
    <small v-if="error.traceId">trace_id：{{ error.traceId }}</small>
    <button v-if="error.retryable" type="button" @click="$emit('retry')">重试</button>
  </section>
</template>
```

`EmptyState.vue`:

```vue
<script setup>
defineProps({ title: { type: String, required: true }, description: { type: String, required: true }, actionLabel: { type: String, default: '' } })
defineEmits(['action'])
</script>
<template>
  <section class="empty-state"><h2>{{ title }}</h2><p>{{ description }}</p><button v-if="actionLabel" type="button" @click="$emit('action')">{{ actionLabel }}</button></section>
</template>
```

`LoadingState.vue`:

```vue
<script setup>
defineProps({ label: { type: String, default: '正在加载' }, rows: { type: Number, default: 3 } })
</script>
<template>
  <div class="loading-state" role="status" aria-live="polite">
    <span>{{ label }}</span>
    <div v-for="row in rows" :key="row" class="loading-row" :style="{ width: `${92 - row * 8}%` }" />
  </div>
</template>
```

`ModeBanner.vue`:

```vue
<script setup>
defineProps({ environment: { type: String, required: true }, provenance: { type: String, required: true }, reason: { type: String, default: '' } })
</script>
<template>
  <aside v-if="environment !== 'production' || provenance !== 'real'" class="mode-banner" role="status">
    {{ environment }} · {{ provenance }}<template v-if="reason"> · {{ reason }}</template>
  </aside>
</template>
```

`ConfirmAction.vue`:

```vue
<script setup>
defineProps({
  open: { type: Boolean, required: true },
  title: { type: String, required: true },
  impact: { type: String, required: true },
  confirmLabel: { type: String, default: '确认' },
  busy: { type: Boolean, default: false },
})
defineEmits(['confirm', 'cancel'])
</script>
<template>
  <div v-if="open" class="confirm-action" role="dialog" aria-modal="true" :aria-label="title">
    <h2>{{ title }}</h2><p>{{ impact }}</p>
    <button type="button" :disabled="busy" @click="$emit('cancel')">取消</button>
    <button type="button" :disabled="busy" @click="$emit('confirm')">{{ confirmLabel }}</button>
  </div>
</template>
```

Run: `npx vitest run tests/components/systemState.test.js`
Expected: PASS.

- [ ] **Step 4: Run component/build gate and commit**

Run: `npm run check:web`

```bash
git add src/components/system src/styles/editorial-tokens.css tests/components/systemState.test.js
git commit -m "feat(web): add normalized system state components"
```

---

## Task 3: Build product shell, navigation, focus, and recoverable routing

**Files:**
- Create: `app/web/src/components/workspace/AppShell.vue`
- Create: `app/web/src/components/workspace/PageHeader.vue`
- Create: `app/web/src/components/workspace/RouteErrorBoundary.vue`
- Modify: `app/web/src/components/workspace/SideNav.vue`
- Modify: `app/web/src/components/workspace/WorkspaceHeader.vue`
- Modify: `app/web/src/layout/consumer/index.vue`
- Modify: `app/web/src/router/finance.js`
- Test: `app/web/tests/components/workspaceShell.test.js`

**Interfaces:**
- Consumes: normalized capability state and route metadata.
- Produces: `AppShell` slots (`header`, `default`), `PageHeader` props (`title`, `summary`, `actions`), and route focus behavior.

- [ ] **Step 1: Write shell behavior tests**

Create `app/web/tests/components/workspaceShell.test.js`:

```js
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AppShell from '../../src/components/workspace/AppShell.vue'
import PageHeader from '../../src/components/workspace/PageHeader.vue'
import RouteErrorBoundary from '../../src/components/workspace/RouteErrorBoundary.vue'

describe('workspace shell', () => {
  it('exposes skip link and moves focus to page heading', async () => {
    const wrapper = mount(AppShell, { attachTo: document.body, slots: { default: '<main id="main-content"><h1 id="page-title">期货概览</h1></main>' } })
    expect(wrapper.find('a[href="#main-content"]').text()).toContain('跳到主要内容')
    wrapper.vm.focusPageTitle()
    await wrapper.vm.$nextTick()
    expect(document.activeElement.id).toBe('page-title')
  })

  it('renders a recoverable route error with trace ID', () => {
    const wrapper = mount(RouteErrorBoundary, { props: { error: { code: 'ROUTE_NOT_FOUND', message: '页面不存在', traceId: 'trace-route' } } })
    expect(wrapper.text()).toContain('返回工作台')
    expect(wrapper.text()).toContain('trace-route')
  })

  it('uses one primary action in the page header', () => {
    const wrapper = mount(PageHeader, { props: { title: '品种', summary: '事实与合约', primaryAction: { label: '发起研究' }, secondaryActions: [{ label: '导出' }] } })
    expect(wrapper.findAll('button').filter(node => node.classes().includes('primary')).length).toBe(1)
  })
})
```

Run: `npx vitest run tests/components/workspaceShell.test.js`
Expected: FAIL because components are missing.

- [ ] **Step 2: Implement AppShell and focus contract**

Create `app/web/src/components/workspace/AppShell.vue` with responsive CSS grid, `SkipLink`, `SideNav`, `WorkspaceHeader`, a main region with `id="main-content"`, and `focusPageTitle()` that focuses `#page-title`. The component props are `navigation`, `user`, `capabilities`, `environment`; it emits `toggle-navigation`, `logout`, and `navigate`. Include a nav drawer for `<768px`, close it after navigation, and return focus to the menu button.

- [ ] **Step 3: Implement PageHeader and RouteErrorBoundary**

`PageHeader` props: `title`, `summary`, `primaryAction`, `secondaryActions`, `status`. It renders exactly one `.primary` button and sets `tabindex="-1"` on the `<h1 id="page-title">`. `RouteErrorBoundary` props: `error`, `homeLabel`; it always renders a link to `/app`, prints stable `error.code`, `error.traceId`, and only emits `retry` for retryable errors.

- [ ] **Step 4: Extend existing navigation and layout**

Modify `SideNav.vue` to consume normalized groups `{label, items:[{to,label,badge,disabled,reason}]}` and close the mobile drawer after `navigate`. Modify `WorkspaceHeader.vue` to show `pageTitle`, `environment`, `provenance`, capability reason, and account menu. Modify `layout/consumer/index.vue` to use `AppShell` and route metadata rather than page-specific hardcoded navigation.

- [ ] **Step 5: Add router title/focus and 404 recovery**

Modify `app/web/src/router/finance.js` so every route has `meta.title`, `meta.module`, and optional `meta.primaryAction`. Add a catch-all route that renders `RouteErrorBoundary` with `ROUTE_NOT_FOUND` and a link to `/app`. Add an `afterEach` hook that sets `document.title` and focuses `#page-title` after `nextTick`, preserving the requested URL for login redirects.

Run: `npx vitest run tests/components/workspaceShell.test.js && npm run build`
Expected: tests and build pass.

- [ ] **Step 6: Commit**

```bash
git add src/components/workspace src/layout/consumer/index.vue src/router/finance.js tests/components/workspaceShell.test.js
git commit -m "feat(web): add product shell and recoverable routing"
```

---

## Task 4: Build futures typed client, pure model, and concurrency store

**Files:**
- Create: `app/web/src/api/futures.js`
- Modify: `app/web/src/modules/futures/api.js`
- Create: `app/web/src/modules/futures/model.js`
- Modify: `app/web/src/modules/futures/store.js`
- Test: `app/web/tests/unit/futuresModel.test.js`
- Test: `app/web/tests/unit/futuresStore.test.js`

**Interfaces:**
- Consumes: Task 1 `unwrapEnvelope`, `normalizePage`, `runRequest`, `createRequestSequence`.
- Produces: `fetchCapabilities`, `fetchProducts`, `fetchProduct`, `createResearch`, `fetchRun`, `cancelRun`, `fetchHypothesis`, `listNotifications`, `batchReadNotifications`, `createSourceProbe`, `patchOperations`, plus store actions with idempotency keys.

- [ ] **Step 1: Write pure model tests**

Create `app/web/tests/unit/futuresModel.test.js` covering capability block reasons, product contract kind (`actual`, `continuous`, `dominant`), freshness/stale calculation, run state transitions (`queued/running/verifying/succeeded/failed/canceled` plus local `canceling`), notification filters, and source-probe state (`PASS/FAIL/NOT_RUN/BLOCKED`). Use fixture objects copied from `contracts/futures/v1`; do not call the network.

- [ ] **Step 2: Implement pure model normalization**

Create `app/web/src/modules/futures/model.js` with named exports `normalizeCapabilities`, `normalizeProduct`, `normalizeContracts`, `normalizeRun`, `normalizeHypothesis`, `normalizeNotification`, `normalizeSource`, `canSubmitResearch`, and `runDisplayState`. Pure functions must never mutate input and must preserve `traceId`, source/version/time fields, and backend state strings.

- [ ] **Step 3: Write store race and idempotency tests**

Create `app/web/tests/unit/futuresStore.test.js` with mocked `api/futures.js`. Assert: stale responses are discarded after route/account invalidation; two rapid submissions reuse one idempotency key until settled; retry generates a new key; notifications batch-read updates optimistically and restores all affected rows on failure; polling stops after `succeeded/failed/canceled`; `canceling` is local until server state returns.

- [ ] **Step 4: Implement typed API client**

Create `app/web/src/api/futures.js`. Export one function per backend route with explicit query/body objects, strict enum checks, `Idempotency-Key` for mutations, `AbortSignal`, and `unwrapEnvelope`/`normalizePage`. Never build URLs by concatenating untrusted strings; use `encodeURIComponent`. `secret`, `credential_ref`, and bearer/session tokens are never accepted as response view models.

- [ ] **Step 5: Implement Pinia store**

Rewrite `app/web/src/modules/futures/store.js` around resource maps `{status,data,error,requestedAt,sequence}`. Add `resetForAccount`, `loadOverview`, `loadProduct`, `startResearch`, `loadRun`, `pollRun`, `cancelRun`, `loadHypothesis`, `loadNotifications`, `batchReadNotifications`, `loadSources`, `runSourceProbe`, and `activateLive`. Use `createRequestSequence`, abort controllers, idempotency-key factories, and a poll interval of 3000 ms that stops on terminal run state.

Run: `npx vitest run tests/unit/futuresModel.test.js tests/unit/futuresStore.test.js`
Expected: PASS.

- [ ] **Step 6: Run build and commit**

```bash
git add src/api/futures.js src/modules/futures tests/unit/futuresModel.test.js tests/unit/futuresStore.test.js
git commit -m "feat(web): add futures client model and request state"
```

---

## Task 5: Implement futures overview and product workbench

**Files:**
- Create: `app/web/src/modules/futures/pages/OverviewPage.vue`
- Create: `app/web/src/modules/futures/pages/ProductPage.vue`
- Modify: `app/web/src/modules/futures/routes.js`
- Test: `app/web/tests/components/futuresOverview.test.js`

**Interfaces:**
- Consumes: Task 2 system components and Task 4 store/model functions.
- Produces: route-ready overview and product views with detail links.

- [ ] **Step 1: Write overview/product behavior tests**

Mount both pages with mocked stores. Assert overview ordering: mode/blocking capability, freshness/failure/notifications, watchlist, recent runs, active hypotheses. Every summary links to its detail route. Product page distinguishes `dominant`, `continuous`, and `actual` contracts, renders data-as-of and timezone, and shows a blocking reason before research submission when coverage is insufficient.

- [ ] **Step 2: Implement OverviewPage**

Use `PageHeader`, `CapabilityStatus`, `DataFreshness`, `ModeBanner`, `EmptyState`, and `LoadingState`. The first viewport must show module mode and blocking capability plus freshness/failures/notifications. Summary cards use semantic headings and links. Do not render internal metrics or full reports.

- [ ] **Step 3: Implement ProductPage**

Render title metadata (`name`, `exchange`, `product_id`, timezone, `as_of`), a segmented contract filter, separate tables for dominant/continuous/actual contracts, market/term-structure/inventory/source-coverage/research sections, and an `发起研究` action carrying product and source context. No data means `EmptyState` with request range and last success time.

- [ ] **Step 4: Register lazy routes and test responsive constraints**

Add `/app/futures` and `/app/futures/products/:productId` to `modules/futures/routes.js` with title/module metadata. Component tests at 320 px width assert no fixed horizontal table container; tables are wrapped in `overflow-x:auto` regions with accessible labels.

Run: `npx vitest run tests/components/futuresOverview.test.js && npm run build`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/modules/futures/pages/OverviewPage.vue src/modules/futures/pages/ProductPage.vue src/modules/futures/routes.js tests/components/futuresOverview.test.js
git commit -m "feat(web): add futures overview and product workbench"
```

---

## Task 6: Implement three-stage research creation and run detail

**Files:**
- Create: `app/web/src/modules/futures/pages/NewResearchPage.vue`
- Create: `app/web/src/modules/futures/pages/ResearchDetailPage.vue`
- Create: `app/web/src/components/system/FileUpload.vue`
- Test: `app/web/tests/components/futuresResearch.test.js`

**Interfaces:**
- Consumes: Task 4 `startResearch`, `loadRun`, `pollRun`, `cancelRun`; `SafeSourceLink`; system components.
- Produces: idempotent submission and server-state-driven run detail.

- [ ] **Step 1: Write research-flow tests**

Test three in-page stages without new wizard routes. Upload accepts `.pdf/.txt/.md`, rejects files over 20 MiB, shows progress, allows cancel and per-file retry. Submission displays product/question/sources/models/budget/gaps, keeps one idempotency key across network retry, and generates a new key after explicit user retry. Detail renders every server run state, local `canceling`, elapsed time, budget, report sections, evidence links, export, and failure `trace_id`.

- [ ] **Step 2: Implement FileUpload**

Wrap a native input with a visible keyboard-operable button, `aria-describedby` limits, drag/drop as an enhancement only, per-file status (`queued/uploading/failed/succeeded`), cancellation, and Object URL cleanup on unmount. Never store file contents in Pinia.

- [ ] **Step 3: Implement NewResearchPage**

Use three visible sections with `aria-current="step"`: `问题与材料`, `解析与核对`, `范围确认`. Persist draft fields in memory only. Show parse conflicts, source/time, and per-material retry. Disable final submit when capability is blocking or material parsing fails without an accepted exclusion.

- [ ] **Step 4: Implement ResearchDetailPage**

Render timeline states from the backend; use `canceling` only as a local transition. Keep completed stages and diagnostics on failure. Report rendering accepts only structured fields and fixed sections (`coverage_summary`, `conclusion`, `claim_reviews`, `counter_evidence`, `conditions`, `gaps`, `versions`). Evidence drawer shows quote, source, time, rights, claim association, and verification result.

- [ ] **Step 5: Register routes, run tests, and commit**

Add `/app/futures/research/new` and `/app/futures/research/:runId`.

```bash
git add src/components/system/FileUpload.vue src/modules/futures/pages/NewResearchPage.vue src/modules/futures/pages/ResearchDetailPage.vue src/modules/futures/routes.js tests/components/futuresResearch.test.js
git commit -m "feat(web): add futures research creation and run detail"
```

---

## Task 7: Implement hypotheses, notifications, and source administration

**Files:**
- Create: `app/web/src/modules/futures/pages/HypothesisPage.vue`
- Create: `app/web/src/modules/futures/pages/NotificationsPage.vue`
- Create: `app/web/src/modules/futures/pages/AdminSourcesPage.vue`
- Test: `app/web/tests/components/futuresOperations.test.js`

**Interfaces:**
- Consumes: Task 2 `ConfirmAction`, Task 4 store actions, backend pagination/probe/gate contracts.
- Produces: lifecycle, batch-read rollback, source probe and release-gate UI.

- [ ] **Step 1: Write lifecycle/notification/admin tests**

Hypothesis tests cover domain states and readable condition expressions; unavailable calculations show `waiting/blocked`, never `not_triggered`. Notification tests cover server filters `read/type/object_id/object_version`, optimistic batch-read, full rollback, and deep links. Admin tests cover strict source form fields, secret fields write-only, four-state probe results, gate expiry/digest/ID display, CAS expected version, and delete-impact confirmation.

- [ ] **Step 2: Implement HypothesisPage**

Render proposition, direction, confidence, observation window, assumptions, counterevidence, source run, lifecycle, validation conditions, and recap. Lifecycle actions emit service operations with expected version and display impact in `ConfirmAction`. Disabled actions include reason text.

- [ ] **Step 3: Implement NotificationsPage**

Filters map exactly to server query fields. Batch selection is capped at 100 and rejects duplicates. On batch-read failure restore `read_at` for every optimistic row and keep a persistent `RequestError`. Each row contains time, object, severity, reason, and target link.

- [ ] **Step 4: Implement AdminSourcesPage**

Use Element Plus controls only. Structured fields cover publisher, URL, rights, metrics, mapping, schedule, pagination, coverage, limits, retries, adapter and responsible role. `credential_ref` is write-only. Probe result supports `PASS/FAIL/NOT_RUN/BLOCKED` and displays evidence hash/config digest/secret revision/fingerprint. `live` activation requires gate ID, digest, expected version, reason, and confirmation.

- [ ] **Step 5: Register routes, run tests, and commit**

Add `/app/futures/hypotheses/:id`, `/app/futures/notifications`, and `/admin/futures/sources`.

```bash
git add src/modules/futures/pages/HypothesisPage.vue src/modules/futures/pages/NotificationsPage.vue src/modules/futures/pages/AdminSourcesPage.vue src/modules/futures/routes.js tests/components/futuresOperations.test.js
git commit -m "feat(web): add futures hypothesis notification and source workflows"
```

---

## Task 8: Regress research, strategy, intelligence, and shared styling

**Files:**
- Modify: `app/web/src/view/research/new.vue`
- Modify: `app/web/src/view/research/detail.vue`
- Modify: `app/web/src/view/strategies/index.vue`
- Modify: `app/web/src/view/strategies/market.vue`
- Modify: `app/web/src/view/strategies/marketDetail.vue`
- Modify: `app/web/src/view/intel/index.vue`
- Modify: `app/web/src/styles/consumer-semi.css`
- Test: `app/web/tests/regression/existingModules.test.js`

**Interfaces:**
- Consumes: Tasks 1–2 shared error/status components.
- Produces: no changed backend calls; consistent freshness/error/empty semantics across existing modules.

- [ ] **Step 1: Write regression tests before edits**

Mount key existing pages with fixture view models and assert: research submission disabled without real model; report body ≥16 px; strategy editor does not rewrite input; stale chart displays last timestamp and gap reason; intelligence environment and provenance use separate labels; all `SafeSourceLink` external links use safe attributes; no fixture banner in production state.

- [ ] **Step 2: Replace duplicated state markup**

Use `DataFreshness`, `RequestError`, `EmptyState`, `LoadingState`, and `ModeBanner` in the listed pages without changing their stores or API calls. Preserve existing Element Plus admin pages and Semi UI consumer pages.

- [ ] **Step 3: Normalize shared CSS**

Extend `consumer-semi.css` for 12-column desktop, 8-column tablet, single-column mobile; 240/208 px sidebars; content max width 1440 px; 14–16 px body; 16 px report text; table overflow wrappers; visible focus; 200% zoom-safe controls. Remove only CSS rules replaced by tokens.

- [ ] **Step 4: Run regression and build**

Run: `npx vitest run tests/regression/existingModules.test.js && npm run build`
Expected: PASS; existing Node route tests remain green.

- [ ] **Step 5: Commit**

```bash
git add src/view/research src/view/strategies src/view/intel src/styles/consumer-semi.css tests/regression/existingModules.test.js
git commit -m "refactor(web): unify state presentation across existing modules"
```

---

## Task 9: Add E2E, accessibility, responsive, storage, and failure tests

**Files:**
- Create: `app/web/e2e/core-journeys.spec.js`
- Create: `app/web/e2e/futures-journeys.spec.js`
- Create: `app/web/e2e/accessibility.spec.js`
- Create: `app/web/e2e/responsive.spec.js`
- Create: `app/web/e2e/security-and-failures.spec.js`
- Modify: `app/web/playwright.config.js`
- Test fixtures: `app/web/e2e/fixtures/*.json`

**Interfaces:**
- Consumes: fixture API routes through Playwright `page.route`; production build from Task 8.
- Produces: ten core journey results and FPR-27–FPR-33/FPR-39 evidence.

- [ ] **Step 1: Configure deterministic Playwright**

Pin Chromium, Firefox, and WebKit projects at 1440x900, 1024x768, 390x844, and 320x568. Set `trace: 'retain-on-failure'`, `screenshot: 'only-on-failure'`, `video: 'retain-on-failure'`, and a 30-second navigation timeout. Fixture APIs must use route interception and a per-test cache namespace.

- [ ] **Step 2: Add ten core journeys**

Cover login/safe redirect, research create/detail, strategy edit/market, intelligence event/evidence, futures overview/product, futures research create/run cancel/recover, hypothesis lifecycle, notification batch rollback, source probe/gate rejection, and recoverable 404. Assert no empty route, no placeholder button, and no fixture label in production state.

- [ ] **Step 3: Add accessibility and keyboard tests**

Use `axe-core` injected into each core page and fail on `critical` or `serious` issues. Complete the futures research journey with keyboard only. Assert skip link, heading focus after route change, dialog focus return, and visible focus. Test 200% and 400% zoom using browser scaling or CSS pixel viewport equivalents.

- [ ] **Step 4: Add responsive and visual snapshots**

At 1440/1024/390/320 assert `document.documentElement.scrollWidth <= clientWidth`. Snapshot only overview, product, research detail, report/evidence, notifications, and source admin after animations are disabled. Review snapshots and store them under `app/web/e2e/__screenshots__`.

- [ ] **Step 5: Add storage and failure injection**

Failures include 401 session expiry, 403 role denial, 404, 409 revision/idempotency, 429 retry, 503 module unavailable, malformed envelope, stale response after account switch, partial upload failure, notification rollback, probe `FAIL`, expired gate, and cancellation. After all tests assert `localStorage` and `sessionStorage` contain no `token`, `jwt`, `session`, `csrf`, or secret-like values.

Run: `npx playwright test`
Expected: all normal tests pass; no hidden skips.

- [ ] **Step 6: Commit**

```bash
git add e2e playwright.config.js
git commit -m "test(web): add product journeys accessibility and failure evidence"
```

---

## Task 10: Performance budget, release evidence, and final audit

**Files:**
- Create: `app/web/tests/performance/bundle-budget.test.js`
- Create: `app/scripts/verify-frontend.sh`
- Create: `app/tests/contract/test_frontend_release.py`
- Modify: `app/scripts/release-evidence.py`
- Modify: `app/deploy/README.md`
- Test output: `artifacts/frontend/<version>/manifest.json`

**Interfaces:**
- Consumes: build output, Vitest/Playwright JSON results, contract files, source digests.
- Produces: secret-free frontend release manifest and R0–R2 evidence.

- [ ] **Step 1: Write bundle-budget tests**

Parse `app/web/dist/.vite/manifest.json` or `dist/manifest.json`. Assert initial JS ≤450 KiB gzip, initial CSS ≤80 KiB gzip, route chunks ≤250 KiB gzip, and no source map contains `Bearer`, `__Host-zhigu_session`, private keys, or task credentials. Allow the current large research route to fail until code splitting brings it below 250 KiB.

- [ ] **Step 2: Add frontend verifier**

`verify-frontend.sh` runs `npm ci`, `npm test`, `npm run build`, `npx playwright test`, contract tests, and writes Vitest/Playwright summaries. Missing browser binaries or Docker produces exit 2 and `NOT_RUN`, never PASS.

- [ ] **Step 3: Add deterministic frontend manifest**

Extend `release-evidence.py` with `--frontend-result`, producing `frontend` fields for commit, build hash, bundle budget, tests, skip count, viewport matrix, axe result, storage scan, and known limits. Reject secret-like values. Create `test_frontend_release.py` asserting deterministic output and no credentials.

- [ ] **Step 4: Document deployment and evidence boundaries**

Update `app/deploy/README.md` with frontend build flags, runtime capability gating, cache/security headers, release-manifest command, and the boundary between fixture E2E and real backend/provider verification.

- [ ] **Step 5: Run final gates**

Run:

```bash
app/scripts/verify-frontend.sh
python3 app/scripts/release-evidence.py --output artifacts/frontend/local/manifest.json --version local --frontend-result artifacts/frontend/local/results.json
```

Expected: local automated gates pass or external-only cases explicitly report `NOT_RUN`; no test silently skips.

- [ ] **Step 6: Commit**

```bash
git add tests/performance ../../scripts/verify-frontend.sh ../../tests/contract/test_frontend_release.py ../../scripts/release-evidence.py ../../app/deploy/README.md
git commit -m "feat(web): add frontend release verification and evidence"
```

---

## Completion Evidence

Record each task’s test command, result, commit SHA, browser matrix, and reviewer result in `.superpowers/sdd/frontend-progress.md`. FPR-44–FPR-46 remain deployment gates until real data/model and production browser evidence exist.
