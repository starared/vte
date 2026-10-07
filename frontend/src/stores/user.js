import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '../api'

export const useUserStore = defineStore('user', () => {
  const token = ref(localStorage.getItem('token') || '')
  const user = ref(null)

  const isLoggedIn = computed(() => !!token.value)

  function setToken(value) {
    token.value = value
    localStorage.setItem('token', value)
  }

  async function login(username, password) {
    const res = await api.post('/api/auth/login', { username, password })
    setToken(res.data.access_token)
    await fetchUser()
  }

  async function fetchUser() {
    if (!token.value) return
    try {
      const res = await api.get('/api/auth/me')
      user.value = res.data
    } catch {
      // 401 已由 api 拦截器统一处理（登出并跳转登录页）；
      // 网络抖动等其他错误不应把用户踢下线
    }
  }

  function logout() {
    token.value = ''
    user.value = null
    localStorage.removeItem('token')
  }

  // 初始化时获取用户信息
  if (token.value) {
    fetchUser()
  }

  return { token, user, isLoggedIn, login, setToken, fetchUser, logout }
})
