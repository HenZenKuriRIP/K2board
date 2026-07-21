import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/pages/LoginPage.vue'),
      meta: { requiresAuth: false },
    },
    {
      path: '/',
      component: () => import('@/layouts/AdminLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        { path: '', name: 'Dashboard', component: () => import('@/pages/DashboardPage.vue'), meta: { title: '仪表盘' } },
        { path: '/users', name: 'Users', component: () => import('@/pages/users/UserListPage.vue'), meta: { title: '用户管理' } },
        { path: '/monitor', name: 'OnlineMonitor', component: () => import('@/pages/monitor/OnlineMonitorPage.vue'), meta: { title: '在线监控' } },
        { path: '/nodes', name: 'Nodes', component: () => import('@/pages/nodes/NodeListPage.vue'), meta: { title: '节点管理' } },
        { path: '/groups', name: 'Groups', component: () => import('@/pages/groups/GroupPage.vue'), meta: { title: '权限组管理' } },
        { path: '/plans', name: 'Plans', component: () => import('@/pages/plans/PlanPage.vue'), meta: { title: '订阅计划' } },
        { path: '/orders', name: 'Orders', component: () => import('@/pages/orders/OrderListPage.vue'), meta: { title: '订单管理' } },
        { path: '/payment-methods', name: 'PaymentMethods', component: () => import('@/pages/orders/PaymentMethodPage.vue'), meta: { title: '支付方式' } },
        { path: '/referral', name: 'Referral', component: () => import('@/pages/referral/ReferralPage.vue'), meta: { title: '推广管理' } },
        { path: '/traffic', name: 'Traffic', component: () => import('@/pages/traffic/TrafficLogPage.vue'), meta: { title: '流量分析' } },
        { path: '/queue', name: 'QueueMonitor', component: () => import('@/pages/queue/QueueMonitorPage.vue'), meta: { title: '后台调度' } },
        { path: '/logs', name: 'SystemLog', component: () => import('@/pages/system/SystemLogPage.vue'), meta: { title: '系统日志' } },
        { path: '/settings', name: 'Settings', component: () => import('@/pages/settings/SettingsPage.vue'), meta: { title: '系统设置' } },
      ],
    },
  ],
})

router.beforeEach((to, _from, next) => {
  const token = localStorage.getItem('token')
  if (to.meta.requiresAuth !== false && !token) {
    next('/login')
  } else if (to.path === '/login' && token) {
    next('/')
  } else {
    next()
  }
})

export default router
