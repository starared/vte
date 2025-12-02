<template>
  <div class="providers">
    <div class="header">
      <h2>提供商管理</h2>
      <el-button type="primary" @click="showAdd">添加提供商</el-button>
    </div>

    <el-table :data="providers" v-loading="loading" stripe>
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
          {{ row.provider_type === 'vertex_express' ? 'Vertex Express' : '标准' }}
        </template>
      </el-table-column>
      <el-table-column prop="is_active" label="状态" width="80">
        <template #default="{ row }">
          <el-tag :type="row.is_active ? 'success' : 'info'" size="small">
            {{ row.is_active ? '启用' : '禁用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="380">
        <template #default="{ row }">
          <el-button size="small" @click="fetchModels(row)" :disabled="row.provider_type === 'vertex_express'">拉取模型</el-button>
          <el-button size="small" @click="viewModels(row)">查看模型</el-button>
          <el-button size="small" @click="viewAPIKeys(row)">密钥管理</el-button>
          <el-button size="small" type="success" @click="showTestDialog(row)">测试</el-button>
          <el-button size="small" @click="editProvider(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="deleteProvider(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 添加/编辑对话框 -->
    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑提供商' : '添加提供商'" width="550px" :fullscreen="isMobile">
      <el-form :model="form" label-width="100px">
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
          <div class="form-tip">用户看到的模型名会加上此前缀（如 openai/gpt-4），修改后自动同步到所有模型</div>
        </el-form-item>
        
        <template v-if="form.provider_type === 'standard'">
          <el-form-item label="API 地址" required>
            <el-input v-model="form.base_url" placeholder="https://api.openai.com/v1" />
            <div class="form-tip">完整地址，需包含 /v1（如 https://api.openai.com/v1）</div>
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
        <el-input v-model="newModelId" placeholder="输入模型 ID，如 gpt-4o、gemini-2.5-pro" style="width: 300px" />
        <el-button type="primary" @click="addModel" style="margin-left: 12px">添加模型</el-button>
      </div>
      <el-table :data="providerModels" max-height="400">
        <el-table-column prop="original_id" label="模型 ID" min-width="200" />
        <el-table-column prop="display_name" label="显示名称" min-width="150" />
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
        <el-input v-model="newAPIKey" type="password" show-password placeholder="输入 API Key" style="width: 250px" />
        <el-input v-model="newAPIKeyName" placeholder="密钥名称（可选）" style="width: 150px; margin-left: 8px" />
        <el-button type="primary" @click="addAPIKey" style="margin-left: 12px">添加密钥</el-button>
      </div>
      <el-table :data="providerAPIKeys" max-height="400" empty-text="暂无密钥，请添加">
        <el-table-column prop="name" label="名称" width="120" />
        <el-table-column prop="api_key" label="密钥" width="200">
          <template #default="{ row }">
            <div class="key-display">
              <span>{{ keyVisibility[row.id] ? row.api_key : maskKey(row.api_key) }}</span>
              <el-icon class="eye-icon" @click="toggleKeyVisibility(row.id)">
                <component :is="keyVisibility[row.id] ? 'Hide' : 'View'" />
              </el-icon>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="usage_count" label="使用次数" width="90" />
        <el-table-column prop="last_used_at" label="最后使用" width="160">
          <template #default="{ row }">
            {{ row.last_used_at ? new Date(row.last_used_at).toLocaleString() : '从未使用' }}
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
      <el-form label-width="80px">
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
        <div class="test-info">模型: {{ testResult.model }}</div>
        <div class="test-info">密钥: {{ testResult.api_key_name }}</div>
        <div class="test-info">耗时: {{ testResult.duration_ms }}ms</div>
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
import { ref, computed, onMounted, reactive } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { View, Hide } from '@element-plus/icons-vue'
import api from '../api'

const loading = ref(false)
const saving = ref(false)
const providers = ref([])
const dialogVisible = ref(false)
const modelsDialogVisible = ref(false)
const apiKeysDialogVisible = ref(false)
const testDialogVisible = ref(false)
const editingId = ref(null)
const currentProvider = ref(null)
const providerModels = ref([])
const providerAPIKeys = ref([])
const newModelId = ref('')
const newAPIKey = ref('')
const newAPIKeyName = ref('')
const testing = ref(false)
const testResult = ref(null)
const testOptions = ref({ models: [], api_keys: [] })
const testForm = ref({ modelId: null, apiKeyId: null })
const isMobile = computed(() => window.innerWidth < 768)

// 密钥显示/隐藏控制
const keyVisibility = reactive({})

function maskKey(key) {
  if (!key) return ''
  return '••••••••••••••••'
}

function toggleKeyVisibility(keyId) {
  keyVisibility[keyId] = !keyVisibility[keyId]
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
  is_active: true
})

async function loadProviders() {
  loading.value = true
  try {
    const res = await api.get('/api/providers')
    providers.value = res.data
  } finally {
    loading.value = false
  }
}

function showAdd() {
  editingId.value = null
  form.value = { 
    name: '', base_url: '', api_key: '', model_prefix: '',
    provider_type: 'standard', vertex_project: '', vertex_location: 'global',
    proxy_url: '', is_active: true 
  }
  dialogVisible.value = true
}

async function editProvider(row) {
  editingId.value = row.id
  form.value = { ...row, api_key: '' }
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
  } finally {
    saving.value = false
  }
}

async function deleteProvider(row) {
  await ElMessageBox.confirm('确定删除该提供商？关联的模型也会被删除', '确认')
  await api.delete(`/api/providers/${row.id}`)
  ElMessage.success('删除成功')
  loadProviders()
}

async function fetchModels(row) {
  try {
    const res = await api.post(`/api/providers/${row.id}/fetch-models`)
    ElMessage.success(res.data.message)
  } catch {}
}

async function viewModels(row) {
  currentProvider.value = row
  const res = await api.get(`/api/providers/${row.id}/models`)
  providerModels.value = res.data
  modelsDialogVisible.value = true
}

async function toggleModel(row) {
  await api.put(`/api/models/${row.id}`, { is_active: row.is_active })
}

async function addModel() {
  if (!newModelId.value) {
    ElMessage.warning('请输入模型 ID')
    return
  }
  try {
    await api.post(`/api/providers/${currentProvider.value.id}/add-model`, { model_id: newModelId.value })
    ElMessage.success('添加成功')
    newModelId.value = ''
    viewModels(currentProvider.value)
  } catch {}
}

async function deleteModel(row) {
  await ElMessageBox.confirm('确定删除该模型？', '确认')
  try {
    await api.delete(`/api/models/${row.id}`)
    ElMessage.success('删除成功')
    viewModels(currentProvider.value)
  } catch {}
}

// API Keys 管理
async function viewAPIKeys(row) {
  currentProvider.value = row
  try {
    const res = await api.get(`/api/providers/${row.id}/api-keys`)
    providerAPIKeys.value = res.data || []
    apiKeysDialogVisible.value = true
  } catch (e) {
    ElMessage.error('获取密钥列表失败')
    console.error(e)
  }
}

async function addAPIKey() {
  if (!newAPIKey.value) {
    ElMessage.warning('请输入 API Key')
    return
  }
  try {
    await api.post(`/api/providers/${currentProvider.value.id}/api-keys`, {
      api_key: newAPIKey.value,
      name: newAPIKeyName.value
    })
    ElMessage.success('添加成功')
    newAPIKey.value = ''
    newAPIKeyName.value = ''
    viewAPIKeys(currentProvider.value)
  } catch {}
}

async function toggleAPIKey(row) {
  await api.put(`/api/providers/${currentProvider.value.id}/api-keys/${row.id}`, { is_active: row.is_active })
}

async function deleteAPIKey(row) {
  await ElMessageBox.confirm('确定删除该密钥？', '确认')
  try {
    await api.delete(`/api/providers/${currentProvider.value.id}/api-keys/${row.id}`)
    ElMessage.success('删除成功')
    viewAPIKeys(currentProvider.value)
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
    const res = await api.post(`/api/providers/${currentProvider.value.id}/test`, {
      model_id: testForm.value.modelId || undefined,
      api_key_id: testForm.value.apiKeyId || undefined
    })
    testResult.value = res.data
  } catch (e) {
    testResult.value = {
      success: false,
      message: e.response?.data?.detail || '请求失败'
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
}
.no-prefix {
  color: #c0c4cc;
}
.key-row {
  display: flex;
  align-items: center;
  margin-bottom: 8px;
}
.model-actions {
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}

.key-display {
  display: flex;
  align-items: center;
  gap: 8px;
}
.key-display span {
  font-family: monospace;
  font-size: 13px;
}
.eye-icon {
  cursor: pointer;
  color: var(--el-text-color-secondary);
  font-size: 16px;
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
}
.test-error {
  color: var(--el-color-danger);
  margin-top: 8px;
  word-break: break-all;
}

@media (max-width: 768px) {
  .header h2 { font-size: 18px; }
  .model-actions .el-input { width: 100% !important; }
  .model-actions .el-button { margin-left: 0 !important; }
}
</style>
