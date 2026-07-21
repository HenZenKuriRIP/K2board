import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/user/login',
      name: 'UserLogin',
      component: () => import('@/pages/UserLoginPage.vue'),
    },
    {
      path: '/user/register',
      name: 'UserRegister',
      component: () => import('@/pages/UserRegisterPage.vue'),
    },
    {
      path: '/user/forgot-password',
      name: 'UserForgotPassword',
      component: () => import('@/pages/UserForgotPasswordPage.vue'),
    },
    {
      path: '/user',
      component: () => import('@/layouts/UserLayout.vue'),
      children: [
        { path: '', name: 'UserDashboard', component: () => import('@/pages/UserDashboard.vue'), meta: { title: '仪表盘' } },
        { path: 'subscribe', name: 'UserSubscribe', component: () => import('@/pages/UserSubscribe.vue'), meta: { title: '订阅' } },
        { path: 'docs', name: 'UserDocs', component: () => import('@/pages/UserDocs.vue'), meta: { title: '使用教程' } },
        { path: 'orders', name: 'UserOrders', component: () => import('@/pages/UserOrders.vue'), meta: { title: '我的订单' } },
        { path: 'orders/:trade_no', name: 'UserOrderPay', component: () => import('@/pages/UserOrderPay.vue'), meta: { title: '订单支付' } },
        { path: 'profile', name: 'UserProfile', component: () => import('@/pages/UserProfile.vue'), meta: { title: '个人中心' } },
        { path: 'referral', name: 'UserReferral', component: () => import('@/pages/UserReferral.vue'), meta: { title: '推广返佣' } },
        { path: 'order-result', name: 'UserOrderResult', component: () => import('@/pages/UserOrderResult.vue'), meta: { title: '支付结果' } },
      ],
    },
    // Default redirect
    { path: '/', redirect: '/user' },
    { path: '/:pathMatch(.*)*', redirect: '/user' },
  ],
})

router.beforeEach((to, _from, next) => {
  const userToken = localStorage.getItem('user_token')

  // Only skip login if a token is present. Token validity is checked by /user/info;
  // invalid tokens are cleared there. Allow explicit re-login via query ?force=1
  const authPublic =
    to.path === '/user/login' ||
    to.path === '/user/register' ||
    to.path === '/user/forgot-password'
  if (authPublic && userToken) {
    if (to.query.force === '1') {
      localStorage.removeItem('user_token')
      localStorage.removeItem('user_email')
      next()
      return
    }
    // 找回密码允许已登录用户进入（force 或直接访问）
    if (to.path === '/user/forgot-password') {
      next()
      return
    }
    next('/user')
    return
  }

  next()
})

export default router
