import { createRouter, createWebHistory } from 'vue-router'
import ConsumerLayout from '../layout/consumer/index.vue'
import AdminLayout from '../layout/admin/index.vue'
import IntelLayout from '../layout/intel/index.vue'

const intelEnabled = import.meta.env.VITE_INTEL_ENABLED === 'true'
const intelMeta = (mode, tab, extra = {}) => ({ module: 'intel', mode, intelTab: tab, ...extra })
const intelRoutes = intelEnabled
  ? [{
      path: '/app/intel',
      component: IntelLayout,
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

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/app/research/new' },
    { path: '/login', component: () => import('../view/research/login.vue') },
    ...intelRoutes,
    {
      path: '/app',
      component: ConsumerLayout,
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
      path: '/admin',
      component: AdminLayout,
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

router.beforeEach((to) => {
  const token = localStorage.getItem('zhigu_token')
  const role = localStorage.getItem('zhigu_role')
  const demoIntel = to.meta?.module === 'intel' && to.meta?.mode === 'demo'
  if (to.path !== '/login' && !token && !demoIntel) {
    return { path: '/login', query: { redirect: safeInternalRedirect(to.fullPath, '/app/research/new') } }
  }
  if (to.meta?.requiresAdmin && role !== 'admin') return '/app/research/new'
  if (to.path.startsWith('/admin') && role !== 'admin') return '/app/research/new'
})

export default router
