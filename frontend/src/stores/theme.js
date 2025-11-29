import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import api from '../api'

export const useThemeStore = defineStore('theme', () => {
  const theme = ref(localStorage.getItem('theme') || 'light')
  const isLoggedIn = ref(false)

  // 应用主题
  function applyTheme(newTheme) {
    const html = document.documentElement
    
    if (newTheme === 'auto') {
      // 跟随系统
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
      html.classList.toggle('dark', prefersDark)
    } else {
      html.classList.toggle('dark', newTheme === 'dark')
    }
    
    localStorage.setItem('theme', newTheme)
  }

  // 监听主题变化
  watch(theme, (newTheme) => {
    applyTheme(newTheme)
  }, { immediate: true })

  // 监听系统主题变化
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
    if (theme.value === 'auto') {
      document.documentElement.classList.toggle('dark', e.matches)
    }
  })

  // 从服务器加载主题设置
  async function loadTheme() {
    try {
      const res = await api.get('/api/settings/theme')
      theme.value = res.data.theme
      applyTheme(theme.value)
    } catch (e) {
      // 使用本地存储的主题
      applyTheme(theme.value)
    }
  }

  // 保存主题设置到服务器
  async function setTheme(newTheme) {
    theme.value = newTheme
    applyTheme(newTheme)
    try {
      await api.put('/api/settings/theme', { theme: newTheme })
    } catch (e) {
      console.error('保存主题设置失败', e)
    }
  }

  // 切换主题
  function toggleTheme() {
    const themes = ['light', 'dark', 'auto']
    const currentIndex = themes.indexOf(theme.value)
    const nextIndex = (currentIndex + 1) % themes.length
    setTheme(themes[nextIndex])
  }

  return { theme, loadTheme, setTheme, toggleTheme }
})
