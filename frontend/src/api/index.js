import axios from 'axios'
import { ElMessage } from 'element-plus'

const api = axios.create({
  baseURL: '',
  timeout: 30000
})

// 请求拦截器
api.interceptors.request.use(config => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

let handlingUnauthorized = false

async function handleUnauthorized() {
  if (handlingUnauthorized) return
  handlingUnauthorized = true
  try {
    // 动态导入，避免 api ↔ store ↔ router 的循环依赖
    const [{ useUserStore }, { default: router }] = await Promise.all([
      import('../stores/user'),
      import('../router')
    ])
    useUserStore().logout()
    if (router.currentRoute.value.path !== '/login') {
      ElMessage.warning('登录已过期，请重新登录')
      await router.replace('/login')
    }
  } finally {
    handlingUnauthorized = false
  }
}

// 响应拦截器
// 统一在这里提示错误；页面里的 catch 不需要再弹一次。
// 个别请求不想弹提示时，可传 { silent: true }。
api.interceptors.response.use(
  response => response,
  error => {
    const status = error.response?.status
    const isLoginRequest = error.config?.url?.includes('/api/auth/login')

    if (status === 401 && !isLoginRequest) {
      handleUnauthorized()
      return Promise.reject(error)
    }

    if (!error.config?.silent) {
      const msg = error.response?.data?.error?.message || error.response?.data?.detail || error.message || '请求失败'
      ElMessage.error(msg)
    }
    return Promise.reject(error)
  }
)

export default api
