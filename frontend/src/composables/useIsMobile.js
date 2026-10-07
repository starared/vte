import { ref } from 'vue'

// 与各页面 CSS 中的 @media (max-width: 768px) 保持一致
const query = '(max-width: 768px)'
const mql = typeof window !== 'undefined' ? window.matchMedia(query) : null
const isMobile = ref(mql ? mql.matches : false)

if (mql) {
  const update = e => { isMobile.value = e.matches }
  if (mql.addEventListener) {
    mql.addEventListener('change', update)
  } else {
    mql.addListener(update) // 旧版 Safari
  }
}

/** 全局共享的「是否窄屏」响应式状态，窗口大小变化时自动更新 */
export function useIsMobile() {
  return isMobile
}
