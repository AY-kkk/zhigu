import { createRouter, createWebHistory } from 'vue-router'
import ConsumerLayout from '../layout/consumer/index.vue'
import AdminLayout from '../layout/admin/index.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/app/research/new' },
    { path: '/login', component: () => import('../view/research/login.vue') },
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
        { path: 'strategies', component: () => import('../view/strategies/index.vue') }
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

router.beforeEach((to) => {
  const token = localStorage.getItem('zhigu_token')
  if (to.path !== '/login' && !token) return '/login'
  if (to.path.startsWith('/admin') && localStorage.getItem('zhigu_role') !== 'admin') return '/app/research/new'
})

export default router
