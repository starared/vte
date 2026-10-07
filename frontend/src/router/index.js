import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '../stores/user'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('../views/Login.vue')
  },
  {
    path: '/',
    component: () => import('../views/Layout.vue'),
    meta: { requiresAuth: true },
    children: [
      { path: '', redirect: '/dashboard' },
      { path: 'dashboard', name: 'Dashboard', meta: { title: '仪表盘' }, component: () => import('../views/Dashboard.vue') },
      { path: 'providers', name: 'Providers', meta: { title: '提供商' }, component: () => import('../views/Providers.vue') },
      { path: 'models', name: 'Models', meta: { title: '模型管理' }, component: () => import('../views/Models.vue') },
      { path: 'temp-keys', name: 'TempKeys', meta: { title: '临时 API' }, component: () => import('../views/TempKeys.vue') },
      { path: 'token-stats', name: 'TokenStats', meta: { title: 'Token 统计' }, component: () => import('../views/TokenStats.vue') },
      { path: 'settings', name: 'Settings', meta: { title: '设置' }, component: () => import('../views/Settings.vue') },
      { path: 'about', name: 'About', meta: { title: '关于' }, component: () => import('../views/About.vue') }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.afterEach(to => {
  document.title = to.meta.title ? `${to.meta.title} · VTE` : 'VTE'
})

router.beforeEach((to, from, next) => {
  const userStore = useUserStore()
  if (to.meta.requiresAuth && !userStore.isLoggedIn) {
    next('/login')
  } else if (to.path === '/login' && userStore.isLoggedIn) {
    next('/')
  } else {
    next()
  }
})

export default router
