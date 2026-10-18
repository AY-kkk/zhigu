import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'
import AppShell from '../../src/components/workspace/AppShell.vue'
import PageHeader from '../../src/components/workspace/PageHeader.vue'
import RouteErrorBoundary from '../../src/components/workspace/RouteErrorBoundary.vue'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/', redirect: '/app' }, { path: '/app', component: { template: '<div />' } }],
})

describe('workspace shell', () => {
  it('exposes skip link and moves focus to page heading', async () => {
    const wrapper = mount(AppShell, {
      attachTo: document.body,
      slots: {
        default: '<main><h1 id="page-title">期货概览</h1></main>',
      },
    })
    expect(wrapper.find('a[href="#main-content"]').text()).toContain('跳到主要内容')
    wrapper.vm.focusPageTitle()
    await wrapper.vm.$nextTick()
    expect(document.activeElement.id).toBe('page-title')
  })

  it('renders a recoverable route error with trace ID', () => {
    const wrapper = mount(RouteErrorBoundary, {
      global: { plugins: [router] },
      props: {
        error: {
          code: 'ROUTE_NOT_FOUND',
          message: '页面不存在',
          traceId: 'trace-route',
        },
      },
    })
    expect(wrapper.text()).toContain('返回工作台')
    expect(wrapper.text()).toContain('trace-route')
  })

  it('uses one primary action in the page header', () => {
    const wrapper = mount(PageHeader, {
      props: {
        title: '品种',
        summary: '事实与合约',
        primaryAction: { label: '发起研究' },
        secondaryActions: [{ label: '导出' }],
      },
    })
    expect(wrapper.findAll('button').filter(node => node.classes().includes('primary')).length).toBe(1)
  })
})
