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
      <el-table-column label="操作" width="280">
        <template #default="{ row }">
          <el-button size="small" @click="fetchModels(row)" :disabled="row.provider_type === 'vertex_express'">拉取模型</el-button>
          <el-button size="small" @click="viewModels(row)">查看模型</el-button>
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
        
        <el-form-item label="API Key" :required="!editingId">
          <el-input v-model="form.api_key" type="password" show-password 
            :placeholder="editingId ? '留空则保持原值' : (form.provider_type === 'vertex_express' ? 'Vertex Express API Key' : 'API Key')" />
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
      <div class="model-actions" v-if="currentProvider?.provider_type === 'vertex_express'">
        <el-input v-model="newModelId" placeholder="输入模型 ID，如 gemini-2.5-pro" style="width: 300px" />
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
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const loading = ref(false)
const saving = ref(false)
const providers = ref([])
const dialogVisible = ref(false)
const modelsDialogVisible = ref(false)
const editingId = ref(null)
const currentProvider = ref(null)
const providerModels = ref([])
const newModelId = ref('')
const isMobile = computed(() => window.innerWidth < 768)

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

function editProvider(row) {
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
  color: #909399;
  margin-top: 4px;
}
.no-prefix {
  color: #c0c4cc;
}
.model-actions {
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}

@media (max-width: 768px) {
  .header h2 { font-size: 18px; }
  .model-actions .el-input { width: 100% !important; }
  .model-actions .el-button { margin-left: 0 !important; }
}
</style>
