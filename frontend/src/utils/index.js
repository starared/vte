import { ElMessage, ElMessageBox } from 'element-plus'

/**
 * 复制文本到剪贴板。
 * navigator.clipboard 只在 HTTPS 或 localhost 下可用，
 * 通过 http://IP:8050 访问时需要退回到 execCommand('copy')。
 */
export async function copyText(text) {
  if (!text) return false
  let ok = false
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      ok = true
    } catch {
      ok = false
    }
  }
  if (!ok) ok = legacyCopy(text)

  if (ok) {
    ElMessage.success('已复制')
  } else {
    ElMessage.error('复制失败，请手动复制')
  }
  return ok
}

function legacyCopy(text) {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.top = '-9999px'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  textarea.setSelectionRange(0, text.length) // iOS Safari
  let ok = false
  try {
    ok = document.execCommand('copy')
  } catch {
    ok = false
  }
  document.body.removeChild(textarea)
  return ok
}

/**
 * 二次确认。用户点「取消」或关闭弹窗时返回 false，而不是抛出异常，
 * 避免产生未捕获的 Promise rejection。
 */
export async function confirmAction(message, title = '确认', options = {}) {
  try {
    await ElMessageBox.confirm(message, title, {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
      ...options
    })
    return true
  } catch {
    return false
  }
}

export const MASKED_SECRET = '••••••••••••••••'

export function formatDateTime(value) {
  if (!value) return ''
  return new Date(value).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

export function formatNumber(num) {
  if (!num) return '0'
  return num.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',')
}
