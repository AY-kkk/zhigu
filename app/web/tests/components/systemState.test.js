import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CapabilityStatus from '../../src/components/system/CapabilityStatus.vue'
import DataFreshness from '../../src/components/system/DataFreshness.vue'
import RequestError from '../../src/components/system/RequestError.vue'

describe('system state components', () => {
  it('shows all four dimensions and non-color status text', () => {
    const wrapper = mount(CapabilityStatus, {
      props: { state: {
        deploymentEnvironment: 'production',
        operationMode: 'read_only',
        dataProvenance: 'real',
        health: 'unavailable',
        reason: 'MODEL_UNAVAILABLE',
        blocking: true,
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
      props: {
        asOf: '2026-10-13T10:00:00Z',
        updatedAt: '2026-10-13T10:05:00Z',
        stale: true,
        reason: 'UPSTREAM_DELAY',
      },
    })
    expect(wrapper.text()).toContain('数据截止')
    expect(wrapper.text()).toContain('更新')
    expect(wrapper.text()).toContain('陈旧')
    expect(wrapper.attributes('role')).toBe('status')
  })

  it('keeps recoverable errors on the page with trace ID', () => {
    const wrapper = mount(RequestError, {
      props: {
        error: {
          code: 'UPSTREAM_TIMEOUT',
          message: '请求超时',
          retryable: true,
          traceId: 'trace-1',
        },
      },
    })
    expect(wrapper.text()).toContain('UPSTREAM_TIMEOUT')
    expect(wrapper.text()).toContain('trace-1')
    expect(wrapper.find('button').exists()).toBe(true)
  })
})
