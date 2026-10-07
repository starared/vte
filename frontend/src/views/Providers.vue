<template>
  <div class="providers">
    <div class="header">
      <h2>提供商管理</h2>
      <el-button type="primary" @click="showAdd">添加提供商</el-button>
    </div>

    <!-- 桌面端：表格 -->
    <el-table v-if="!isMobile" :data="providers" v-loading="loading" stripe>
      <el-table-column prop="name" label="名称" width="150" />
      <el-table-column prop="model_prefix" label="模型前缀" width="120">
        <template #default="{ row }">
          <el-tag v-if="row.model_prefix" size="small">{{ row.model_prefix }}</el-tag>
          <span v-else class="no-prefix">无</span>
        </template>
      </el-table-column>
      <el-table-column prop="base_url" label="API 地址" min-width="250" show-overflow-tooltip />
      <el-table-column prop="provider_type" label="类型" width="140">
        <template #default="{ row }">
          {{ providerTypeLabel(row) }}
        </template>
      </el-table-column>
      <el-table-column prop="is_active" label="状态" width="80">
        <template #default="{ row }">
          <el-tag :type="row.is_active ? 'success' : 'info'" size="small">
            {{ row.is_active ? '启用' : '禁用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="240" fixed="right">
        <template #default="{ row }">
          <div class="row-actions">
            <el-button size="small" @click="viewModels(row)">模型</el-button>
            <el-button size="small" type="success" plain @click="showTestDialog(row)">测试</el-button>
            <el-dropdown trigger="click" @command="cmd => handleCommand(cmd, row)">
              <el-button size="small">
                更多<el-icon class="el-icon--right"><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="fetch" :disabled="!canFetch(row)">拉取模型</el-dropdown-item>
                  <el-dropdown-item command="keys">密钥管理</el-dropdown-item>
                  <el-dropdown-item command="edit">编辑</el-dropdown-item>
                  <el-dropdown-item command="delete" divided class="danger-item">删除</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <!-- 移动端：卡片列表 -->
    <div v-else class="provider-cards" v-loading="loading">
      <el-empty v-if="!loading && providers.length === 0" description="还没有提供商" :image-size="80" />
      <el-card v-for="row in providers" :key="row.id" class="provider-card" shadow="never">
        <div class="card-top">
          <div class="card-title">
            <span class="name">{{ row.name }}</span>
            <el-tag v-if="row.model_prefix" size="small">{{ row.model_prefix }}</el-tag>
          </div>
          <el-tag :type="row.is_active ? 'success' : 'info'" size="small">
            {{ row.is_active ? '启用' : '禁用' }}
          </el-tag>
        </div>
        <div class="card-meta">{{ providerTypeLabel(row) }}</div>
        <div v-if="row.base_url" class="card-url">{{ row.base_url }}</div>
        <div class="card-actions">
          <el-button size="small" @click="viewModels(row)">模型</el-button>
          <el-button size="small" type="success" plain @click="showTestDialog(row)">测试</el-button>
          <el-dropdown trigger="click" @command="cmd => handleCommand(cmd, row)">
            <el-button size="small">
              更多<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="fetch" :disabled="!canFetch(row)">拉取模型</el-dropdown-item>
                <el-dropdown-item command="keys">密钥管理</el-dropdown-item>
                <el-dropdown-item command="edit">编辑</el-dropdown-item>
                <el-dropdown-item command="delete" divided class="danger-item">删除</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-card>
    </div>

    <!-- 添加/编辑对话框 -->
    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑提供商' : '添加提供商'" width="550px" :fullscreen="isMobile">
      <el-form :model="form" label-width="100px" :label-position="isMobile ? 'top' : 'right'">
        <el-form-item label="类型" required>
          <el-radio-group v-model="form.provider_type">
            <el-radio value="standard">标准 OpenAI 兼容</el-radio>
            <el-radio value="vertex_express">Vertex Express</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="如: OpenAI, Vertex" />
        </el-form-item>
        <el-form-item label="模型前缀">
          <el-input v-model="form.model_prefix" placeholder="如: openai、vertex，用于区分来源" />
          <div class="form-tip">用户看到的模型名会加上此前缀（如 openai/gpt-4）。修改后自动同步到所有未自定义名称的模型，临时 API 和限流规则中的引用也会一起更新</div>
        </el-form-item>
        
        <template v-if="form.provider_type === 'standard'">
          <el-form-item label="API 地址" required>
            <el-input v-model="form.base_url" placeholder="https://api.openai.com/v1" />
            <div class="form-tip">填写服务商的基础地址（如 https://api.openai.com/v1），不要附加 /chat/completions</div>
          </el-form-item>
        </template>
        
        <template v-if="form.provider_type === 'vertex_express'">
          <el-form-item label="项目编号" required>
            <el-input v-model="form.vertex_project" placeholder="GCP 项目编号" />
          </el-form-item>
          <el-form-item label="区域">
            <el-input v-model="form.vertex_location" placeholder="默认 global" />
          </el-form-item>
        </template>
        
        <el-form-item v-if="!editingId" label="API Key" required>
          <el-input v-model="form.api_key" type="password" show-password 
            :placeholder="form.provider_type === 'vertex_express' ? 'Vertex Express API Key' : 'API Key'" />
          <div class="form-tip">添加后可在「密钥管理」中管理多个密钥</div>
        </el-form-item>
        <el-form-item label="代理地址">
          <el-input v-model="form.proxy_url" placeholder="可选，如: http://127.0.0.1:7890" />
        </el-form-item>
        <el-form-item label="额外请求头">
          <el-input
            v-model="form.extra_headers"
            type="textarea"
            :rows="3"
            placeholder='可选，JSON 对象，如: {"HTTP-Referer": "https://example.com"}'
          />
          <div class="form-tip">每次请求上游时附带这些请求头，值必须是字符串</div>
        </el-form-item>
        <el-form-item label="状态" v-if="editingId">
          <el-switch v-model="form.is_active" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveProvider" :loading="saving">保存</el-button>
      </template>
    </el-dialog>

    <!-- 模型列表对话框 -->
    <el-dialog v-model="modelsDialogVisible" :title="`${currentProvider?.name} 的模型`" width="700px" :fullscreen="isMobile">
      <div class="model-actions">
        <el-input v-model="newModelId" placeholder="输入模型 ID，如 gpt-4o、gemini-2.5-pro" class="action-input" @keyup.enter="addModel" />
        <el-button type="primary" @click="addModel">添加模型</el-button>
        <el-button v-if="canFetch(currentProvider)" :loading="fetchingId === currentProvider?.id" @click="fetchModels(currentProvider)">从上游拉取</el-button>
      </div>
      <el-table :data="providerModels" max-height="400" v-loading="modelsLoading" empty-text="暂无模型">
        <el-table-column prop="original_id" label="模型 ID" min-width="200">
          <template #default="{ row }">
            <div class="model-id-cell">
              <span>{{ row.original_id }}</span>
              <el-tag v-if="row.source === 'manual'" size="small" type="info">手动</el-tag>
              <el-tag v-if="row.disabled_by_sync" size="small" type="danger">上游已下线</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column v-if="!isMobile" prop="display_name" label="显示名称" min-width="150" />
        <el-table-column prop="is_active" label="状态" width="100">
          <template #default="{ row }">
            <el-switch v-model="row.is_active" @change="toggleModel(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80">
          <template #default="{ row }">
            <el-button size="small" type="danger" @click="deleteModel(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- API Keys 管理对话框 -->
    <el-dialog v-model="apiKeysDialogVisible" :title="`${currentProvider?.name} 的密钥管理`" width="750px" :fullscreen="isMobile">
      <div class="form-tip" style="margin-bottom: 16px">
        添加多个密钥后，每次请求会自动轮换使用启用的密钥，实现负载均衡。
      </div>
      <div class="model-actions">
        <el-input v-model="newAPIKey" type="password" show-password placeholder="输入 API Key" class="action-input" />
        <el-input v-model="newAPIKeyName" placeholder="密钥名称（可选）" class="action-input-sm" />
        <el-button type="primary" @click="addAPIKey">添加密钥</el-button>
      </div>
      <el-table :data="providerAPIKeys" max-height="400" empty-text="暂无密钥，请添加">
        <el-table-column prop="name" label="名称" min-width="100" />
        <el-table-column prop="api_key" label="密钥" min-width="180">
          <template #default="{ row }">
            <div class="key-display">
              <span>{{ keyVisibility[row.id] ? row.api_key : maskKey(row.api_key) }}</span>
              <el-icon class="eye-icon" @click="toggleKeyVisibility(row.id)">
                <component :is="keyVisibility[row.id] ? Hide : View" />
              </el-icon>
            </div>
          </template>
        </el-table-column>
        <el-table-column v-if="!isMobile" prop="usage_count" label="使用次数" width="90" />
        <el-table-column v-if="!isMobile" prop="last_used_at" label="最后使用" width="160">
          <template #default="{ row }">
            {{ row.last_used_at ? formatDateTime(row.last_used_at) : '从未使用' }}
          </template>
        </el-table-column>
        <el-table-column prop="is_active" label="状态" width="70">
          <template #default="{ row }">
            <el-switch v-model="row.is_active" @change="toggleAPIKey(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80">
          <template #default="{ row }">
            <el-button size="small" type="danger" @click="deleteAPIKey(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- 测试连接对话框 -->
    <el-dialog v-model="testDialogVisible" :title="`测试 ${currentProvider?.name} 连接`" width="500px" :fullscreen="isMobile">
      <el-form label-width="80px" :label-position="isMobile ? 'top' : 'right'">
        <el-form-item label="模型">
          <el-select v-model="testForm.modelId" placeholder="选择模型（默认第一个）" clearable style="width: 100%">
            <el-option v-for="m in testOptions.models" :key="m.id" :label="m.display_name" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="密钥">
          <el-select v-model="testForm.apiKeyId" placeholder="选择密钥（默认）" clearable style="width: 100%">
            <el-option v-for="k in testOptions.api_keys" :key="k.id" :label="k.name" :value="k.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <div v-if="testResult" class="test-result" :class="{ success: testResult.success, error: !testResult.success }">
        <div class="test-status">{{ testResult.success ? '✓ 连接成功' : '✗ 连接失败' }}</div>
        <div v-if="testResult.model" class="test-info">模型: {{ testResult.model }}</div>
        <div v-if="testResult.api_key_name" class="test-info">密钥: {{ testResult.api_key_name }}</div>
        <div v-if="testResult.duration_ms != null" class="test-info">耗时: {{ testResult.duration_ms }}ms</div>
        <div v-if="testResult.response" class="test-info">响应: {{ testResult.response }}</div>
        <div v-if="!testResult.success" class="test-error">{{ testResult.message }}</div>
      </div>
      <template #footer>
        <el-button @click="testDialogVisible = false">关闭</el-button>
        <el-button type="primary" @click="runTest" :loading="testing">测试连接</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, reactive } from 'vue'
import { ElMessage } from 'element-plus'
import { View, Hide, ArrowDown } from '@element-plus/icons-vue'
import api from '../api'
import { useIsMobile } from '../composables/useIsMobile'
import { confirmAction, formatDateTime, MASKED_SECRET } from '../utils'

const isMobile = useIsMobile()
const loading = ref(false)
const saving = ref(false)
const providers = ref([])
const dialogVisible = ref(false)
const modelsDialogVisible = ref(false)
const modelsLoading = ref(false)
const apiKeysDialogVisible = ref(false)
const testDialogVisible = ref(false)
const editingId = ref(null)
const currentProvider = ref(null)
const providerModels = ref([])
const providerAPIKeys = ref([])
const newModelId = ref('')
const newAPIKey = ref('')
const newAPIKeyName = ref('')
const fetchingId = ref(null)
const testing = ref(false)
const testResult = ref(null)
const testOptions = ref({ models: [], api_keys: [] })
const testForm = ref({ modelId: null, apiKeyId: null })

// 密钥显示/隐藏控制
const keyVisibility = reactive({})

function maskKey(key) {
  return key ? MASKED_SECRET : ''
}

function toggleKeyVisibility(keyId) {
  keyVisibility[keyId] = !keyVisibility[keyId]
}

function providerTypeLabel(row) {
  return row.provider_type === 'vertex_express' ? 'Vertex Express' : '标准 OpenAI 兼容'
}

function canFetch(row) {
  return !!row && row.provider_type !== 'vertex_express'
}

function handleCommand(cmd, row) {
  switch (cmd) {
    case 'fetch': return fetchModels(row)
    case 'keys': return viewAPIKeys(row)
    case 'edit': return editProvider(row)
    case 'delete': return deleteProvider(row)
  }
}

const form = ref({
  name: '',
  base_url: '',
  api_key: '',
  model_prefix: '',
  provider_type: 'standard',
  vertex_project: '',
  vertex_location: 'global',
  proxy_url: '',
  extra_headers: '',
  is_active: true
})

// 额外请求头必须是 { "名称": "字符串值" } 形式的 JSON 对象
function validateExtraHeaders(text) {
  if (!text.trim()) return ''
  let parsed
  try {
    parsed = JSON.parse(text)
  } catch {
    return '额外请求头不是有效的 JSON'
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return '额外请求头必须是 JSON 对象'
  }
  if (Object.values(parsed).some(v => typeof v !== 'string')) {
    return '额外请求头的值必须都是字符串'
  }
  return ''
}

async function loadProviders() {
  loading.value = true
  try {
    const res = await api.get('/api/providers')
    providers.value = res.data || []
  } catch {
    // 错误提示已由拦截器处理
  } finally {
    loading.value = false
  }
}

function showAdd() {
  editingId.value = null
  form.value = {
    name: '', base_url: '', api_key: '', model_prefix: '',
    provider_type: 'standard', vertex_project: '', vertex_location: 'global',
    proxy_url: '', extra_headers: '', is_active: true
  }
  dialogVisible.value = true
}

function editProvider(row) {
  editingId.value = row.id
  form.value = { ...row, proxy_url: row.proxy_url || '', extra_headers: row.extra_headers || '', api_key: '' }
  dialogVisible.value = true
}

async function saveProvider() {
  if (!form.value.name) {
    ElMessage.warning('请填写名称')
    return
  }
  if (form.value.provider_type === 'standard' && !form.value.base_url) {
    ElMessage.warning('请填写 API 地址')
    return
  }
  if (form.value.provider_type === 'vertex_express' && !form.value.vertex_project) {
    ElMessage.warning('请填写项目编号')
    return
  }
  if (!editingId.value && !form.value.api_key) {
    ElMessage.warning('请填写 API Key')
    return
  }
  const headersError = validateExtraHeaders(form.value.extra_headers || '')
  if (headersError) {
    ElMessage.warning(headersError)
    return
  }
  form.value.extra_headers = (form.value.extra_headers || '').trim()
  saving.value = true
  try {
    if (editingId.value) {
      await api.put(`/api/providers/${editingId.value}`, form.value)
    } else {
      await api.post('/api/providers', form.value)
    }
    ElMessage.success('保存成功')
    dialogVisible.value = false
    loadProviders()
  } catch {
    // 错误提示已由拦截器处理
  } finally {
    saving.value = false
  }
}

async function deleteProvider(row) {
  if (!(await confirmAction(`确定删除提供商「${row.name}」？关联的模型也会被删除`))) return
  try {
    await api.delete(`/api/providers/${row.id}`)
    ElMessage.success('删除成功')
    loadProviders()
  } catch {}
}

async function fetchModels(row) {
  if (fetchingId.value) return
  fetchingId.value = row.id
  try {
    const res = await api.post(`/api/providers/${row.id}/fetch-models`, null, { timeout: 120000 })
    ElMessage.success(res.data.message || '拉取完成')
    if (modelsDialogVisible.value && currentProvider.value?.id === row.id) {
      loadProviderModels(row)
    }
  } catch {
  } finally {
    fetchingId.value = null
  }
}

async function loadProviderModels(row) {
  modelsLoading.value = true
  try {
    const res = await api.get(`/api/providers/${row.id}/models`)
    providerModels.value = res.data || []
  } catch {
  } finally {
    modelsLoading.value = false
  }
}

function viewModels(row) {
  currentProvider.value = row
  providerModels.value = []
  modelsDialogVisible.value = true
  loadProviderModels(row)
}

async function toggleModel(row) {
  try {
    await api.put(`/api/models/${row.id}`, { is_active: row.is_active })
  } catch {
    row.is_active = !row.is_active // 失败时回滚开关
  }
}

async function addModel() {
  const id = newModelId.value.trim()
  if (!id) {
    ElMessage.warning('请输入模型 ID')
    return
  }
  try {
    await api.post(`/api/providers/${currentProvider.value.id}/add-model`, { model_id: id })
    ElMessage.success('添加成功')
    newModelId.value = ''
    loadProviderModels(currentProvider.value)
  } catch {}
}

async function deleteModel(row) {
  if (!(await confirmAction('确定删除该模型？'))) return
  try {
    await api.delete(`/api/models/${row.id}`)
    ElMessage.success('删除成功')
    loadProviderModels(currentProvider.value)
  } catch {}
}

// API Keys 管理
async function loadAPIKeys(row) {
  try {
    const res = await api.get(`/api/providers/${row.id}/api-keys`)
    providerAPIKeys.value = res.data || []
    return true
  } catch {
    return false
  }
}

async function viewAPIKeys(row) {
  currentProvider.value = row
  if (await loadAPIKeys(row)) {
    apiKeysDialogVisible.value = true
  }
}

async function addAPIKey() {
  if (!newAPIKey.value.trim()) {
    ElMessage.warning('请输入 API Key')
    return
  }
  try {
    await api.post(`/api/providers/${currentProvider.value.id}/api-keys`, {
      api_key: newAPIKey.value.trim(),
      name: newAPIKeyName.value
    })
    ElMessage.success('添加成功')
    newAPIKey.value = ''
    newAPIKeyName.value = ''
    loadAPIKeys(currentProvider.value)
  } catch {}
}

async function toggleAPIKey(row) {
  try {
    await api.put(`/api/providers/${currentProvider.value.id}/api-keys/${row.id}`, { is_active: row.is_active })
  } catch {
    row.is_active = !row.is_active
  }
}

async function deleteAPIKey(row) {
  if (!(await confirmAction('确定删除该密钥？'))) return
  try {
    await api.delete(`/api/providers/${currentProvider.value.id}/api-keys/${row.id}`)
    ElMessage.success('删除成功')
    loadAPIKeys(currentProvider.value)
  } catch {}
}

// 测试连接
async function showTestDialog(row) {
  currentProvider.value = row
  testResult.value = null
  testForm.value = { modelId: null, apiKeyId: null }
  try {
    const res = await api.get(`/api/providers/${row.id}/test-options`)
    testOptions.value = res.data
    if (!testOptions.value.models || testOptions.value.models.length === 0) {
      ElMessage.warning('没有启用的模型可供测试')
      return
    }
    testDialogVisible.value = true
  } catch {}
}

async function runTest() {
  testing.value = true
  testResult.value = null
  try {
    // 结果直接显示在对话框里，不再额外弹错误提示
    const res = await api.post(`/api/providers/${currentProvider.value.id}/test`, {
      model_id: testForm.value.modelId || undefined,
      api_key_id: testForm.value.apiKeyId || undefined
    }, { silent: true, timeout: 120000 })
    testResult.value = res.data
  } catch (e) {
    testResult.value = {
      success: false,
      message: e.response?.data?.detail || e.response?.data?.error?.message || e.message || '请求失败'
    }
  } finally {
    testing.value = false
  }
}

onMounted(loadProviders)
</script>

<style scoped>
.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
  flex-wrap: wrap;
  gap: 12px;
}
.form-tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
  line-height: 1.5;
}
.no-prefix {
  color: var(--el-text-color-placeholder);
}
.row-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.row-actions .el-button + .el-button {
  margin-left: 0;
}
.danger-item {
  color: var(--el-color-danger);
}
.model-actions {
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.model-actions .el-button + .el-button {
  margin-left: 0;
}
.action-input {
  width: 280px;
}
.action-input-sm {
  width: 160px;
}

/* 移动端卡片 */
.provider-cards {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 80px;
}
.provider-card :deep(.el-card__body) {
  padding: 14px 16px;
}
.card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.card-title .name {
  font-weight: 600;
  font-size: 15px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.card-meta {
  margin-top: 6px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.card-url {
  margin-top: 4px;
  font-family: monospace;
  font-size: 12px;
  color: var(--el-text-color-regular);
  word-break: break-all;
}
.card-actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}
.card-actions > .el-button,
.card-actions > .el-dropdown {
  flex: 1;
}
.card-actions .el-button + .el-button {
  margin-left: 0;
}
.card-actions > .el-dropdown > .el-button {
  width: 100%;
}

.model-id-cell {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  word-break: break-all;
}

.key-display {
  display: flex;
  align-items: center;
  gap: 8px;
}
.key-display span {
  font-family: monospace;
  font-size: 13px;
  word-break: break-all;
}
.eye-icon {
  cursor: pointer;
  color: var(--el-text-color-secondary);
  font-size: 16px;
  flex-shrink: 0;
  transition: color 0.2s;
}
.eye-icon:hover {
  color: var(--el-color-primary);
}

.test-result {
  margin-top: 16px;
  padding: 12px;
  border-radius: 6px;
  font-size: 14px;
}
.test-result.success {
  background: var(--el-color-success-light-9);
  border: 1px solid var(--el-color-success-light-5);
}
.test-result.error {
  background: var(--el-color-danger-light-9);
  border: 1px solid var(--el-color-danger-light-5);
}
.test-status {
  font-weight: bold;
  margin-bottom: 8px;
}
.test-info {
  color: var(--el-text-color-secondary);
  margin: 4px 0;
  word-break: break-all;
}
.test-error {
  color: var(--el-color-danger);
  margin-top: 8px;
  word-break: break-all;
}

@media (max-width: 768px) {
  .header h2 { font-size: 18px; }
  .action-input,
  .action-input-sm { width: 100%; }
  .model-actions .el-button { flex: 1; }
}
</style>
