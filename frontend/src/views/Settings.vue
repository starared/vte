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
          <span class="hint-text inline-hint">上游 5xx 或连接失败时的重试次数（0-10）；密钥被拒绝时会自动换密钥，不占用次数</span>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="section">
      <template #header>访问限制</template>

      <el-form label-width="120px" :label-position="labelPosition">
        <el-form-item label="全局速率限制">
          <div class="limit-block">
            <el-switch v-model="rateLimit.enabled" />
            <template v-if="rateLimit.enabled">
              <div class="inline-row">
                <span class="hint-text">每</span>
                <el-input-number v-model="rateLimit.window" :min="1" :max="31536000" controls-position="right" />
                <span class="hint-text">秒最多</span>
                <el-input-number v-model="rateLimit.max_requests" :min="1" controls-position="right" />
                <span class="hint-text">次请求</span>
              </div>
            </template>
            <span class="hint-text">对所有 API 请求生效（包括临时 API），超出时返回 429</span>
          </div>
        </el-form-item>

        <el-form-item label="全局并发限制">
          <div class="limit-block">
            <el-switch v-model="concurrency.enabled" />
            <div v-if="concurrency.enabled" class="inline-row">
              <span class="hint-text">最多同时处理</span>
              <el-input-number v-model="concurrency.limit" :min="1" controls-position="right" />
              <span class="hint-text">个请求</span>
            </div>
            <span class="hint-text">当前正在处理 {{ concurrency.current }} 个请求</span>
          </div>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" @click="saveLimits" :loading="savingLimits">保存限制设置</el-button>
        </el-form-item>

        <el-divider content-position="left">自定义速率限制规则</el-divider>

        <el-form-item label="规则">
          <div class="rules-container">
            <div v-for="(rule, index) in customRateRules" :key="rule.id" class="custom-rule">
              <div class="custom-rule-head">
                <el-input v-model="rule.name" placeholder="规则名称" class="rule-name-input" />
                <el-switch v-model="rule.enabled" active-text="启用" />
                <el-button type="danger" text @click="customRateRules.splice(index, 1)">删除</el-button>
              </div>
              <div class="inline-row">
                <el-select v-model="rule.provider_id" placeholder="提供商" class="rule-provider">
                  <el-option label="所有提供商" :value="0" />
                  <el-option v-for="p in providerOptions" :key="p.id" :label="p.name" :value="p.id" />
                </el-select>
                <el-select v-model="rule.model_name" placeholder="所有模型" clearable filterable allow-create class="rule-model">
                  <el-option v-for="m in modelOptions" :key="m" :label="m" :value="m" />
                </el-select>
              </div>
              <div class="inline-row">
                <span class="hint-text">每</span>
                <el-input-number v-model="rule.window" :min="1" :max="31536000" controls-position="right" />
                <span class="hint-text">秒最多</span>
                <el-input-number v-model="rule.max_requests" :min="1" controls-position="right" />
                <span class="hint-text">次</span>
              </div>
            </div>
            <el-button type="primary" text @click="addRateRule">+ 添加规则</el-button>
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveCustomRateRules" :loading="savingRateRules">保存规则</el-button>
          <span class="hint-text inline-hint">可以按提供商或模型（显示名称）单独限流</span>
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
const savingLimits = ref(false)
const savingRateRules = ref(false)
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
const rateLimit = ref({ enabled: false, max_requests: 60, window: 60 })
const concurrency = ref({ enabled: false, limit: 10, current: 0 })
const customRateRules = ref([])
const providerOptions = ref([])
const modelOptions = ref([])

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
  loadLimits()
})

async function loadLimits() {
  try {
    const [rateRes, concRes, rulesRes, providersRes, modelsRes] = await Promise.all([
      api.get('/api/settings/rate-limit'),
      api.get('/api/settings/concurrency'),
      api.get('/api/settings/custom-rate-limit'),
      api.get('/api/providers'),
      api.get('/api/models')
    ])
    rateLimit.value = rateRes.data
    concurrency.value = concRes.data
    customRateRules.value = (rulesRes.data.rules || []).map(r => ({
      id: r.id,
      name: r.name || '',
      provider_id: r.provider_id || 0,
      model_name: r.model_name || '',
      max_requests: r.max_requests || 60,
      window: r.window || 60,
      enabled: r.enabled !== false
    }))
    providerOptions.value = (providersRes.data || []).map(p => ({ id: p.id, name: p.name }))
    modelOptions.value = [...new Set((modelsRes.data || []).map(m => m.display_name || m.original_id))]
  } catch {}
}

async function saveLimits() {
  savingLimits.value = true
  try {
    await Promise.all([
      api.put('/api/settings/rate-limit', {
        enabled: rateLimit.value.enabled,
        max_requests: rateLimit.value.max_requests,
        window: rateLimit.value.window
      }),
      api.put('/api/settings/concurrency', {
        enabled: concurrency.value.enabled,
        limit: concurrency.value.limit
      })
    ])
    ElMessage.success('限制设置已保存')
  } catch {
  } finally {
    savingLimits.value = false
  }
}

function addRateRule() {
  const nextId = customRateRules.value.reduce((max, r) => Math.max(max, r.id || 0), 0) + 1
  customRateRules.value.push({
    id: nextId,
    name: `规则 ${nextId}`,
    provider_id: 0,
    model_name: '',
    max_requests: 60,
    window: 60,
    enabled: true
  })
}

async function saveCustomRateRules() {
  savingRateRules.value = true
  try {
    await api.put('/api/settings/custom-rate-limit', {
      rules: customRateRules.value.map(r => ({ ...r, model_name: r.model_name || '' }))
    })
    ElMessage.success('自定义规则已保存')
  } catch {
  } finally {
    savingRateRules.value = false
  }
}

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
    const res = await api.post('/api/auth/change-password', {
      old_password: oldPassword.value,
      new_password: newPassword.value
    })
    // 修改密码后旧令牌全部失效，换成服务器返回的新令牌（其他设备需要重新登录）
    if (res.data?.access_token) userStore.setToken(res.data.access_token)
    ElMessage.success('密码修改成功，其他设备上的登录已失效')
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

.limit-block { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; width: 100%; }
.custom-rule {
  background: var(--el-fill-color-light);
  border-radius: 10px;
  padding: 12px;
  margin-bottom: 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.custom-rule-head { display: flex; align-items: center; gap: 12px; }
.rule-name-input { width: 200px; }
.rule-provider { width: 180px; }
.rule-model { width: 240px; }

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
  .rule-name-input { flex: 1; width: auto; min-width: 0; }
  .rule-provider, .rule-model { width: 100%; }
  .limit-block .inline-row,
  .custom-rule .inline-row { flex-direction: row; flex-wrap: wrap; align-items: center; }
}
</style>
