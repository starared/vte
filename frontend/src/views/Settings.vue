<template>
  <div class="settings">
    <h2>设置</h2>

    <el-card class="section">
      <template #header>外观设置</template>
      
      <el-form label-width="120px" :label-position="labelPosition">
        <el-form-item label="主题模式">
          <el-radio-group v-model="themeMode">
            <el-radio value="light">亮色</el-radio>
            <el-radio value="dark">暗色</el-radio>
            <el-radio value="auto">跟随系统</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="section">
      <template #header>API 设置</template>
      
      <el-form label-width="120px" :label-position="labelPosition">
        <el-form-item label="流式模式">
          <el-radio-group v-model="streamMode" @change="updateStreamMode">
            <el-radio value="auto">自动（跟随请求）</el-radio>
            <el-radio value="force_stream">强制流式</el-radio>
            <el-radio value="force_non_stream">强制非流式</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item>
          <span class="hint-text">自动：跟随客户端；强制模式只改变上游调用方式，返回格式仍遵循客户端请求</span>
        </el-form-item>
        
        <el-form-item label="最大重试次数">
          <el-input-number v-model="maxRetries" :min="0" :max="10" @change="updateRetrySettings" />
          <span class="hint-text inline-hint">API 请求失败时的重试次数（0-10）</span>
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
      
      <el-form label-width="120px" :label-position="labelPosition" v-if="systemPromptEnabled">
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
          <el-button type="primary" @click="updateSystemPrompt" :loading="savingPrompt">保存提示词</el-button>
          <span class="hint-text inline-hint">将在每次请求的 messages 最前面注入</span>
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
      
      <el-form label-width="120px" :label-position="labelPosition" v-if="customErrorEnabled">
        <el-form-item label="响应规则">
          <div class="rules-container">
            <div v-for="(rule, index) in customErrorRules" :key="index" class="rule-item">
              <el-input v-model="rule.keyword" placeholder="错误关键词" class="rule-keyword" />
              <el-input v-model="rule.response" placeholder="自定义响应内容" class="rule-response" />
              <el-button type="danger" text @click="removeRule(index)">删除</el-button>
            </div>
            <el-button type="primary" text @click="addRule">+ 添加规则</el-button>
          </div>
        </el-form-item>
        
        <el-form-item>
          <el-button type="primary" @click="updateCustomError" :loading="savingError">保存规则</el-button>
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
      
      <el-form label-width="100px" :label-position="labelPosition">
        <el-form-item label="用户名">
          <div class="inline-row">
            <el-input v-model="username" class="field-input" autocomplete="username" />
            <el-button type="primary" @click="changeUsername" :loading="savingUsername">
              修改用户名
            </el-button>
          </div>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="section">
      <template #header>修改密码</template>
      
      <el-form label-width="100px" :label-position="labelPosition">
        <el-form-item label="原密码">
          <el-input v-model="oldPassword" type="password" show-password class="field-input" autocomplete="current-password" />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="newPassword" type="password" show-password class="field-input" autocomplete="new-password" />
        </el-form-item>
        <el-form-item label="确认新密码">
          <el-input v-model="confirmPassword" type="password" show-password class="field-input" autocomplete="new-password" @keyup.enter="changePassword" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="changePassword" :loading="savingPassword">修改密码</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="section">
      <template #header>API Key</template>
      
      <el-form label-width="100px" :label-position="labelPosition">
        <el-form-item label="当前 Key">
          <ApiKeyField :value="userStore.user?.api_key" />
        </el-form-item>
        <el-form-item>
          <el-button type="warning" @click="regenerateKey" :loading="regenerating">重新生成 API Key</el-button>
          <span class="warning-text">注意：重新生成后旧 Key 将失效</span>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { useUserStore } from '../stores/user'
import { useThemeStore } from '../stores/theme'
import { useIsMobile } from '../composables/useIsMobile'
import { confirmAction } from '../utils'
import ApiKeyField from '../components/ApiKeyField.vue'
import api from '../api'

const userStore = useUserStore()
const themeStore = useThemeStore()
const isMobile = useIsMobile()
const labelPosition = computed(() => (isMobile.value ? 'top' : 'right'))

// 每个区块独立的保存状态，避免点一个按钮所有按钮一起转圈
const savingPrompt = ref(false)
const savingError = ref(false)
const savingUsername = ref(false)
const savingPassword = ref(false)
const regenerating = ref(false)

const username = ref(userStore.user?.username || '')
const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const streamMode = ref('auto')
const maxRetries = ref(3)
const systemPrompt = ref('')
const systemPromptEnabled = ref(false)
const customErrorEnabled = ref(false)
const customErrorRules = ref([])

// 直接绑定到主题 store，与顶栏的切换按钮保持同步
const themeMode = computed({
  get: () => themeStore.theme,
  set: value => {
    themeStore.setTheme(value)
    ElMessage.success('主题已更新')
  }
})

// 用户信息可能在页面打开后才加载完成
watch(() => userStore.user?.username, name => {
  if (name) username.value = name
})

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
  } catch {
    // 错误提示已由拦截器处理
  }
})

// 以下 catch 都留空：错误提示由 api 拦截器统一弹出，避免重复提示

async function updateStreamMode(mode) {
  try {
    await api.put('/api/settings/stream-mode', { mode })
    ElMessage.success('流式模式已更新')
  } catch {}
}

async function updateRetrySettings(value) {
  if (value === null || value === undefined) return
  try {
    await api.put('/api/settings/retry', { max_retries: value })
    ElMessage.success('重试设置已更新')
  } catch {}
}

async function updateSystemPrompt() {
  savingPrompt.value = true
  try {
    await api.put('/api/settings/system-prompt', {
      prompt: systemPrompt.value,
      enabled: systemPromptEnabled.value
    })
    ElMessage.success('系统提示词已更新')
  } catch {
  } finally {
    savingPrompt.value = false
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
  savingError.value = true
  try {
    await api.put('/api/settings/custom-error', {
      enabled: customErrorEnabled.value,
      rules: validRules
    })
    customErrorRules.value = validRules
    ElMessage.success('自定义错误响应已更新')
  } catch {
  } finally {
    savingError.value = false
  }
}

async function changeUsername() {
  if (!username.value.trim()) {
    ElMessage.warning('请输入用户名')
    return
  }
  savingUsername.value = true
  try {
    await api.post('/api/auth/change-username', { new_username: username.value.trim() })
    ElMessage.success('用户名修改成功')
    userStore.fetchUser()
  } catch {
  } finally {
    savingUsername.value = false
  }
}

async function changePassword() {
  if (!oldPassword.value || !newPassword.value) {
    ElMessage.warning('请输入原密码和新密码')
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  savingPassword.value = true
  try {
    await api.post('/api/auth/change-password', {
      old_password: oldPassword.value,
      new_password: newPassword.value
    })
    ElMessage.success('密码修改成功')
    oldPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
  } catch {
  } finally {
    savingPassword.value = false
  }
}

async function regenerateKey() {
  if (!(await confirmAction('确定重新生成 API Key？旧 Key 将立即失效'))) return
  regenerating.value = true
  try {
    await api.post('/api/auth/regenerate-api-key')
    ElMessage.success('API Key 已重新生成')
    userStore.fetchUser()
  } catch {
  } finally {
    regenerating.value = false
  }
}
</script>

<style scoped>
.settings h2 { margin-bottom: 20px; }
.section { margin-bottom: 20px; }
.warning-text { margin-left: 12px; color: var(--el-color-warning); font-size: 13px; }
.hint-text { color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.6; }
.inline-hint { margin-left: 12px; }
.hint-text code { background: var(--el-fill-color-light); padding: 2px 6px; border-radius: 4px; font-size: 12px; margin: 0 4px; }

.field-input { width: 300px; }
.inline-row { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }

.rules-container { width: 100%; }
.rule-item { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.rule-keyword { width: 180px; }
.rule-response { flex: 1; }

.card-header-with-switch {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}

.disabled-hint {
  padding: 8px 0;
}

@media (max-width: 768px) {
  .settings h2 { font-size: 18px; }
  .section :deep(.el-card__body) { padding: 16px; }
  .section :deep(.el-radio-group) { display: flex; flex-direction: column; align-items: flex-start; }
  .section :deep(.el-radio) { margin-right: 0; }
  .field-input { width: 100%; }
  .inline-row { flex-direction: column; align-items: stretch; width: 100%; }
  .inline-hint { margin-left: 0; display: block; margin-top: 6px; }
  .warning-text { margin-left: 0; margin-top: 8px; display: block; }
  .rule-item { flex-wrap: wrap; padding-bottom: 8px; border-bottom: 1px dashed var(--el-border-color-lighter); }
  .rule-keyword, .rule-response { width: 100%; flex: none; }
}
</style>
