import { createRouter, createWebHistory } from 'vue-router'
import { buildFuturesAdminRoute, buildFuturesRoutes } from '../modules/futures/routes.js'
import RouteErrorBoundary from '../components/workspace/RouteErrorBoundary.vue'
import { fetchCsrf } from '../api/session.js'
import { hasAuthCredential } from '../utils/http.js'

const intelEnabled = import.meta.env.VITE_INTEL_ENABLED === 'true'
const futuresEnabled = import.meta.env.VITE_FUTURES_ENABLED === 'true'
const intelMeta = (mode, tab, extra = {}) => ({ module: 'intel', mode, intelTab: tab, ...extra })
const intelRoutes = intelEnabled
  ? [{
      path: '/app/intel',
      component: () => import('../layout/intel/index.vue'),
      children: [
        { path: '', component: () => import('../view/intel/index.vue'), meta: intelMeta('live', 'events') },
        { path: 'events/:id', component: () => import('../view/intel/index.vue'), meta: intelMeta('live', 'detail') },
        { path: 'watchlist', component: () => import('../view/intel/index.vue'), meta: intelMeta('live', 'watchlist') },
        { path: 'notifications', component: () => import('../view/intel/index.vue'), meta: intelMeta('live', 'notifications') },
        { path: 'admin/reviews', component: () => import('../view/intel/index.vue'), meta: intelMeta('live', 'reviews', { requiresAdmin: true }) },
        { path: 'admin/sources', component: () => import('../view/intel/index.vue'), meta: intelMeta('live', 'sources', { requiresAdmin: true }) },
        { path: 'demo', component: () => import('../view/intel/index.vue'), meta: intelMeta('demo', 'events', { demoEntry: true }) },
        { path: 'demo/events/:id', component: () => import('../view/intel/index.vue'), meta: intelMeta('demo', 'detail') },
        { path: 'demo/watchlist', component: () => import('../view/intel/index.vue'), meta: intelMeta('demo', 'watchlist') },
        { path: 'demo/notifications', component: () => import('../view/intel/index.vue'), meta: intelMeta('demo', 'notifications') }
      ]
    }]
  : []
const futuresRoutes = buildFuturesRoutes(futuresEnabled)
const futuresAdminRoute = buildFuturesAdminRoute(futuresEnabled)

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/app/research/new' },
    { path: '/login', component: () => import('../view/research/login.vue') },
    ...intelRoutes,
    ...futuresRoutes,
    ...(futuresAdminRoute ? [futuresAdminRoute] : []),
    {
      path: '/app',
      component: () => import('../layout/consumer/index.vue'),
      children: [
        { path: 'research/new', component: () => import('../view/research/new.vue') },
        { path: 'research/:id', component: () => import('../view/research/detail.vue') },
        { path: 'history', component: () => import('../view/research/history.vue') },
        {
          path: 'profile',
          component: () => import('../view/profile/index.vue'),
          beforeEnter: (to) => {
            if (to.query.tab !== 'history') return true
            const query = { ...to.query }
            delete query.tab
            return { path: '/app/history', query }
          }
        },
        {
          path: 'strategies',
          component: () => import('../view/strategies/index.vue'),
          meta: { strategySection: 'workspace', scrollMode: 'fixed' }
        },
        {
          path: 'strategies/market',
          component: () => import('../view/strategies/market.vue'),
          meta: { strategySection: 'market', scrollMode: 'page' }
        },
        {
          path: 'strategies/market/:id',
          component: () => import('../view/strategies/marketDetail.vue'),
          meta: { strategySection: 'market', scrollMode: 'page' }
        }
      ]
    },
    {
      path: '/:pathMatch(.*)*',
      component: RouteErrorBoundary,
      props: {
        error: {
          code: 'ROUTE_NOT_FOUND',
          message: '页面不存在或已被移动',
          traceId: '',
        },
      },
    },
    {
      path: '/admin',
      component: () => import('../layout/admin/index.vue'),
      children: [
        { path: '', redirect: '/admin/ai-settings' },
        { path: 'ai-settings', component: () => import('../view/researchAdmin/settings.vue') },
        { path: 'research-runs', component: () => import('../view/researchAdmin/runs.vue') }
      ]
    }
  ]
})

export function safeInternalRedirect(value, fallback = '/app/research/new') {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//') || value.startsWith('/\\')) return fallback
  if (!value.startsWith('/app/') && !value.startsWith('/admin/')) return fallback
  if (value.startsWith('/login') || value.includes('\\') || value.includes('://')) return fallback
  return value
}

router.beforeEach(async (to) => {
  const demoIntel = to.meta?.module === 'intel' && to.meta?.mode === 'demo'
  if (to.path === '/login' || demoIntel) return true
  if (!hasAuthCredential()) {
    try {
      await fetchCsrf()
    } catch {
      return { path: '/login', query: { redirect: safeInternalRedirect(to.fullPath, '/app/research/new') } }
    }
  }
  return true
})

router.afterEach((to) => {
  const title = to.meta?.title || '知股'
  document.title = `${title} · 知股`
  window.setTimeout(() => {
    const target = document.querySelector('#page-title') || document.querySelector('#main-content h1')
    if (target) {
      if (!target.hasAttribute('tabindex')) target.setAttribute('tabindex', '-1')
      target.focus({ preventScroll: true })
    }
  }, 0)
})

window.addEventListener('zhigu:session-expired', () => {
  const current = router.currentRoute.value
  if (current.path === '/login') return
  const redirect = safeInternalRedirect(current.fullPath, '/app/research/new')
  void router.replace({ path: '/login', query: { redirect } })
})

export default router
