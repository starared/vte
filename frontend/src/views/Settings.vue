<template>
  <div class="settings">
    <h2>设置</h2>

    <el-card class="section">
      <template #header>外观设置</template>
      
      <el-form label-width="120px">
        <el-form-item label="主题模式">
          <el-radio-group v-model="themeMode" @change="updateTheme">
            <el-radio value="light">亮色</el-radio>
            <el-radio value="dark">暗色</el-radio>
            <el-radio value="auto">跟随系统</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="section">
      <template #header>API 设置</template>
      
      <el-form label-width="120px">
        <el-form-item label="流式模式">
          <el-radio-group v-model="streamMode" @change="updateStreamMode">
            <el-radio value="auto">自动（跟随请求）</el-radio>
            <el-radio value="force_stream">强制流式</el-radio>
            <el-radio value="force_non_stream">强制非流式</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item>
          <span class="hint-text">自动：根据客户端请求的 stream 参数决定；强制模式会覆盖客户端设置</span>
        </el-form-item>
        
        <el-form-item label="最大重试次数">
          <el-input-number v-model="maxRetries" :min="0" :max="10" @change="updateRetrySettings" />
          <span class="hint-text" style="margin-left: 12px">API 请求失败时的重试次数（0-10）</span>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="section">
      <template #header>账户设置</template>
      
      <el-form label-width="100px">
        <el-form-item label="用户名">
          <el-input v-model="username" style="width: 300px" />
          <el-button type="primary" @click="changeUsername" :loading="saving" style="margin-left: 12px">
            修改用户名
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="section">
      <template #header>修改密码</template>
      
      <el-form label-width="100px">
        <el-form-item label="原密码">
          <el-input v-model="oldPassword" type="password" show-password style="width: 300px" />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="newPassword" type="password" show-password style="width: 300px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="changePassword" :loading="saving">修改密码</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="section">
      <template #header>API Key</template>
      
      <el-form label-width="100px">
        <el-form-item label="当前 Key">
          <div class="api-key-row">
            <el-input :value="showApiKey ? userStore.user?.api_key : '••••••••••••••••••••••••••••••••'" readonly style="width: 400px" />
            <el-button @click="showApiKey = !showApiKey" style="margin-left: 8px">{{ showApiKey ? '隐藏' : '显示' }}</el-button>
            <el-button @click="copy(userStore.user?.api_key)" style="margin-left: 8px">复制</el-button>
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="warning" @click="regenerateKey" :loading="saving">重新生成 API Key</el-button>
          <span class="warning-text">注意：重新生成后旧 Key 将失效</span>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useUserStore } from '../stores/user'
import { useThemeStore } from '../stores/theme'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const userStore = useUserStore()
const themeStore = useThemeStore()
const saving = ref(false)
const username = ref(userStore.user?.username || '')
const oldPassword = ref('')
const newPassword = ref('')
const streamMode = ref('auto')
const maxRetries = ref(3)
const themeMode = ref(themeStore.theme)
const showApiKey = ref(false)

onMounted(async () => {
  try {
    const [streamRes, retryRes] = await Promise.all([
      api.get('/api/settings/stream-mode'),
      api.get('/api/settings/retry')
    ])
    streamMode.value = streamRes.data.mode
    maxRetries.value = retryRes.data.max_retries
    themeMode.value = themeStore.theme
  } catch (e) {
    console.error('获取设置失败', e)
  }
})

async function updateTheme(theme) {
  themeStore.setTheme(theme)
  ElMessage.success('主题已更新')
}

async function updateStreamMode(mode) {
  try {
    await api.put('/api/settings/stream-mode', { mode })
    ElMessage.success('流式模式已更新')
  } catch (e) {
    ElMessage.error('更新失败')
  }
}

async function updateRetrySettings(value) {
  try {
    await api.put('/api/settings/retry', { max_retries: value })
    ElMessage.success('重试设置已更新')
  } catch (e) {
    ElMessage.error('更新失败')
  }
}

async function changeUsername() {
  if (!username.value) {
    ElMessage.warning('请输入用户名')
    return
  }
  saving.value = true
  try {
    await api.post('/api/auth/change-username', { new_username: username.value })
    ElMessage.success('用户名修改成功')
    userStore.fetchUser()
  } finally {
    saving.value = false
  }
}

async function changePassword() {
  if (!oldPassword.value || !newPassword.value) {
    ElMessage.warning('请输入原密码和新密码')
    return
  }
  saving.value = true
  try {
    await api.post('/api/auth/change-password', {
      old_password: oldPassword.value,
      new_password: newPassword.value
    })
    ElMessage.success('密码修改成功')
    oldPassword.value = ''
    newPassword.value = ''
  } finally {
    saving.value = false
  }
}

async function regenerateKey() {
  await ElMessageBox.confirm('确定重新生成 API Key？旧 Key 将立即失效', '确认')
  saving.value = true
  try {
    await api.post('/api/auth/regenerate-api-key')
    ElMessage.success('API Key 已重新生成')
    userStore.fetchUser()
  } finally {
    saving.value = false
  }
}

function copy(text) {
  navigator.clipboard.writeText(text)
  ElMessage.success('已复制')
}
</script>

<style scoped>
.settings h2 { margin-bottom: 20px; }
.section { margin-bottom: 20px; }
.api-key-row { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.warning-text { margin-left: 12px; color: #e6a23c; font-size: 13px; }
.hint-text { color: #909399; font-size: 12px; }

@media (max-width: 768px) {
  .settings h2 { font-size: 18px; }
  .section :deep(.el-form-item__label) { width: 80px !important; }
  .section :deep(.el-input), .section :deep(.el-radio-group) { width: 100% !important; }
  .api-key-row { flex-direction: column; align-items: stretch; }
  .api-key-row .el-input { width: 100% !important; }
  .api-key-row .el-button { margin-left: 0 !important; }
  .warning-text { margin-left: 0; margin-top: 8px; display: block; }
}
</style>
