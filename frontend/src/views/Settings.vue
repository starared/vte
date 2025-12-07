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
      <template #header>
        <div class="card-header-with-switch">
          <span>系统前置提示词</span>
          <el-switch v-model="systemPromptEnabled" @change="updateSystemPrompt" />
        </div>
      </template>
      
      <el-form label-width="120px" v-if="systemPromptEnabled">
        <el-form-item label="提示词内容">
          <el-input
            v-model="systemPrompt"
            type="textarea"
            :rows="6"
            placeholder="输入系统前置提示词..."
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="updateSystemPrompt" :loading="saving">保存提示词</el-button>
          <span class="hint-text" style="margin-left: 12px">将在每次请求的 messages 最前面注入</span>
        </el-form-item>
      </el-form>
      <div v-else class="disabled-hint">
        <span class="hint-text">开启后将在每次请求的 messages 最前面注入系统提示词</span>
      </div>
    </el-card>

    <el-card class="section">
      <template #header>
        <div class="card-header-with-switch">
          <span>自定义错误响应</span>
          <el-switch v-model="customErrorEnabled" @change="updateCustomError" />
        </div>
      </template>
      
      <el-form label-width="120px" v-if="customErrorEnabled">
        <el-form-item label="响应规则">
          <div class="rules-container">
            <div v-for="(rule, index) in customErrorRules" :key="index" class="rule-item">
              <el-input v-model="rule.keyword" placeholder="错误关键词" style="width: 180px" />
              <el-input v-model="rule.response" placeholder="自定义响应内容" style="flex: 1; margin-left: 8px" />
              <el-button type="danger" text @click="removeRule(index)" style="margin-left: 8px">删除</el-button>
            </div>
            <el-button type="primary" text @click="addRule">+ 添加规则</el-button>
          </div>
        </el-form-item>
        
        <el-form-item>
          <el-button type="primary" @click="updateCustomError" :loading="saving">保存规则</el-button>
        </el-form-item>
        
        <el-form-item>
          <div class="hint-text">
            <p>常用关键词：<code>context_length</code> <code>rate_limit</code> <code>quota</code> <code>invalid_api_key</code> <code>overloaded</code></p>
          </div>
        </el-form-item>
      </el-form>
      <div v-else class="disabled-hint">
        <span class="hint-text">开启后，当错误消息匹配关键词时，返回自定义内容而非报错</span>
      </div>
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
const systemPrompt = ref('')
const systemPromptEnabled = ref(false)
const customErrorEnabled = ref(false)
const customErrorRules = ref([])

onMounted(async () => {
  try {
    const [streamRes, retryRes, promptRes, errorRes] = await Promise.all([
      api.get('/api/settings/stream-mode'),
      api.get('/api/settings/retry'),
      api.get('/api/settings/system-prompt'),
      api.get('/api/settings/custom-error')
    ])
    streamMode.value = streamRes.data.mode
    maxRetries.value = retryRes.data.max_retries
    systemPrompt.value = promptRes.data.prompt || ''
    systemPromptEnabled.value = promptRes.data.enabled
    customErrorEnabled.value = errorRes.data.enabled
    customErrorRules.value = errorRes.data.rules || []
    
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

async function updateSystemPrompt() {
  saving.value = true
  try {
    await api.put('/api/settings/system-prompt', {
      prompt: systemPrompt.value,
      enabled: systemPromptEnabled.value
    })
    ElMessage.success('系统提示词已更新')
  } catch (e) {
    ElMessage.error('更新失败')
  } finally {
    saving.value = false
  }
}

function addRule() {
  customErrorRules.value.push({ keyword: '', response: '' })
}

function removeRule(index) {
  customErrorRules.value.splice(index, 1)
}

async function updateCustomError() {
  // 过滤掉空规则
  const validRules = customErrorRules.value.filter(r => r.keyword && r.response)
  saving.value = true
  try {
    await api.put('/api/settings/custom-error', {
      enabled: customErrorEnabled.value,
      rules: validRules
    })
    customErrorRules.value = validRules
    ElMessage.success('自定义错误响应已更新')
  } catch (e) {
    ElMessage.error('更新失败')
  } finally {
    saving.value = false
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
.hint-text ul { margin: 8px 0 0 20px; padding: 0; }
.hint-text li { margin: 4px 0; }
.hint-text code { background: var(--el-fill-color-light); padding: 2px 6px; border-radius: 4px; font-size: 12px; margin: 0 4px; }

.rules-container { width: 100%; }
.rule-item { display: flex; align-items: center; margin-bottom: 8px; }

.custom-rate-limit-container { width: 100%; }
.custom-rate-limit-item { 
  background: var(--el-fill-color-light); 
  padding: 12px; 
  border-radius: 8px; 
  margin-bottom: 12px; 
}
.rule-row { display: flex; align-items: center; flex-wrap: wrap; gap: 4px; }
.rule-name { margin-top: 8px; }

.card-header-with-switch {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}

.disabled-hint {
  padding: 8px 0;
}

.rate-limit-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
}

@media (max-width: 768px) {
  .settings h2 { font-size: 18px; }
  .section :deep(.el-form-item__label) { width: 80px !important; }
  .section :deep(.el-input), .section :deep(.el-radio-group) { width: 100% !important; }
  .api-key-row { flex-direction: column; align-items: stretch; }
  .api-key-row .el-input { width: 100% !important; }
  .api-key-row .el-button { margin-left: 0 !important; }
  .warning-text { margin-left: 0; margin-top: 8px; display: block; }
  .rule-item { flex-wrap: wrap; gap: 8px; }
  .rule-item .el-input { width: 100% !important; margin-left: 0 !important; }
  .custom-rate-limit-item .rule-row { flex-direction: column; align-items: stretch; }
  .custom-rate-limit-item .rule-row .el-select,
  .custom-rate-limit-item .rule-row .el-input,
  .custom-rate-limit-item .rule-row .el-input-number { width: 100% !important; margin-left: 0 !important; }
  .rate-limit-row { flex-direction: column; align-items: stretch; }
  .rate-limit-row .el-input-number,
  .rate-limit-row .el-select { width: 100% !important; margin-left: 0 !important; }
}
</style>
