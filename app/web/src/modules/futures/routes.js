// Host integration happens only after SPEC task M6; importing this is side-effect free.
export function buildFuturesRoutes(enabled = false) {
  if (enabled !== true) return []
  return [{
    path: '/app/futures',
    component: () => import('./layout/FuturesLayout.vue'),
    meta: { module: 'futures' },
    children: [
      { path: '', component: () => import('./pages/OverviewPage.vue'), meta: { module: 'futures', tab: 'home' } },
      { path: 'products/:productId', component: () => import('./pages/ProductPage.vue'), meta: { module: 'futures', tab: 'product' } },
      { path: 'research/new', component: () => import('./pages/NewResearchPage.vue'), meta: { module: 'futures', tab: 'research-new' } },
      { path: 'research/:runId', component: () => import('./pages/ResearchDetailPage.vue'), meta: { module: 'futures', tab: 'research' } },
      { path: 'hypotheses/:id', component: () => import('./pages/HypothesisPage.vue'), meta: { module: 'futures', tab: 'hypothesis' } },
      { path: 'notifications', component: () => import('./pages/NotificationsPage.vue'), meta: { module: 'futures', tab: 'notifications' } }
    ]
  }]
}

export function buildFuturesAdminRoute(enabled = false) {
  if (enabled !== true) return null
  return {
    path: '/admin/futures/sources',
    component: () => import('./pages/AdminSourcesPage.vue'),
    meta: { module: 'futures', requiresAdmin: true, tab: 'admin-sources' }
  }
}
