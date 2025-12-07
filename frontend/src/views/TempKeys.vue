<template>
  <div class="temp-keys">
    <div class="header">
      <h2>临时 API 密钥</h2>
      <div class="actions">
        <el-button type="primary" @click="openCreate">
          <el-icon><Plus /></el-icon>
          新建临时 API
        </el-button>
        <el-button :icon="Refresh" circle @click="loadData" />
      </div>
    </div>

    <el-alert
      type="info"
      show-icon
      :closable="false"
      class="tip"
      title="为外部或短期使用生成受限的 API Key，可指定可用模型、各自次数、总次数、有效期、速率限制和并发限制。"
    />

    <el-table :data="keys" v-loading="loading" border style="width: 100%" size="small">
      <el-table-column prop="name" label="名称" min-width="100">
        <template #default="{ row }">
          {{ row.name || '-' }}
        </template>
      </el-table-column>
      <el-table-column label="Token" min-width="200">
        <template #default="{ row }">
          <div class="token-row">
            <span class="token-text">{{ maskToken(row.token) }}</span>
            <el-button size="small" @click="copy(row.token)">复制</el-button>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag v-if="isExpired(row)" type="danger" size="small">已过期</el-tag>
          <el-tag v-else-if="!row.is_active" type="warning" size="small">已禁用</el-tag>
          <el-tag v-else type="success" size="small">可用</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="可用模型" min-width="160">
        <template #default="{ row }">
          <div class="tag-wrap">
            <el-tag v-for="m in row.allowed_models?.slice(0, 3)" :key="m" size="small">{{ m }}</el-tag>
            <el-tag v-if="row.allowed_models?.length > 3" size="small" type="info">+{{ row.allowed_models.length - 3 }}</el-tag>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="请求次数" width="120">
        <template #default="{ row }">
          <span>{{ row.used_requests }} / {{ row.max_requests > 0 ? row.max_requests : '不限' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="速率/并发" width="140">
        <template #default="{ row }">
          <div v-if="row.rate_limit_count > 0 || row.concurrency_limit > 0">
            <div v-if="row.rate_limit_count > 0">{{ row.rate_limit_count }}次/{{ row.rate_limit_window }}{{ rateLimitUnitLabel(row.rate_limit_unit) }}</div>
            <div v-if="row.concurrency_limit > 0">并发{{ row.concurrency_limit }}</div>
          </div>
          <span v-else>不限</span>
        </template>
      </el-table-column>
      <el-table-column label="有效期" width="160">
        <template #default="{ row }">
          <div v-if="row.expires_at">
            <div>{{ formatTime(row.expires_at) }}</div>
            <div class="expire-duration" v-if="row.expire_duration">
              ({{ row.expire_duration }}{{ unitLabel(row.expire_unit) }})
            </div>
          </div>
          <span v-else>永久</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="140" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" text size="small" @click="openEdit(row)">编辑</el-button>
          <el-popconfirm title="确认删除该临时 API？" @confirm="remove(row)">
            <template #reference>
              <el-button type="danger" text size="small">删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑临时 API' : '新建临时 API'" width="680px" destroy-on-close>
      <el-form :model="form" label-width="100px" label-position="left">
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="可选，用于标识" />
        </el-form-item>

        <el-form-item label="可用模型" required>
          <el-select
            v-model="form.allowed_models"
            multiple
            filterable
            clearable
            placeholder="选择允许使用的模型"
            style="width: 100%"
            @change="onModelsChange"
          >
            <el-option v-for="m in modelOptions" :key="m.value" :label="m.label" :value="m.value" />
          </el-select>
        </el-form-item>

        <el-form-item label="模型次数限制" v-if="form.allowed_models.length > 0">
          <div class="limits">
            <div class="limit-row" v-for="m in form.allowed_models" :key="m">
              <span class="limit-name">{{ m }}</span>
              <el-input-number
                v-model.number="form.model_limits[m]"
                :min="0"
                :step="1"
                controls-position="right"
                placeholder="0 为不限"
              />
              <span class="limit-hint">次 (0为不限)</span>
            </div>
          </div>
        </el-form-item>

        <el-divider content-position="left">请求限制</el-divider>

        <el-form-item label="总请求次数">
          <el-input-number v-model.number="form.max_requests" :min="0" :step="10" controls-position="right" />
          <span class="form-hint">0 为不限制</span>
        </el-form-item>

        <el-form-item label="速率限制">
          <div class="rate-limit-row">
            <el-input-number v-model.number="form.rate_limit_count" :min="0" :step="1" controls-position="right" placeholder="请求数" />
            <span>次 /</span>
            <el-input-number v-model.number="form.rate_limit_window" :min="0" :step="1" controls-position="right" placeholder="时间" />
            <el-select v-model="form.rate_limit_unit" placeholder="单位" style="width: 90px">
              <el-option label="秒" value="seconds" />
              <el-option label="分钟" value="minutes" />
              <el-option label="小时" value="hours" />
            </el-select>
          </div>
          <div class="form-hint">均为0则不限制</div>
        </el-form-item>

        <el-form-item label="并发限制">
          <el-input-number v-model.number="form.concurrency_limit" :min="0" :step="1" controls-position="right" />
          <span class="form-hint">0 为不限制</span>
        </el-form-item>

        <el-divider content-position="left">有效期</el-divider>

        <el-form-item label="有效时长">
          <div class="expire-row">
            <el-input-number v-model.number="form.expire_duration" :min="0" :step="1" controls-position="right" placeholder="时长" />
            <el-select v-model="form.expire_unit" placeholder="单位" style="width: 100px; margin-left: 8px">
              <el-option label="分钟" value="minutes" />
              <el-option label="小时" value="hours" />
              <el-option label="天" value="days" />
            </el-select>
          </div>
          <div class="form-hint">时长为0或不选单位则永久有效</div>
        </el-form-item>

        <el-form-item label="状态">
          <el-switch v-model="form.is_active" active-text="启用" inactive-text="禁用" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import api from '../api'

const loading = ref(false)
const saving = ref(false)
const keys = ref([])
const modelOptions = ref([])
const dialogVisible = ref(false)
const editingId = ref(null)

const form = reactive({
  name: '',
  allowed_models: [],
  model_limits: {},
  max_requests: 0,
  rate_limit_count: 0,
  rate_limit_window: 0,
  rate_limit_unit: 'seconds',
  concurrency_limit: 0,
  expire_duration: 0,
  expire_unit: '',
  is_active: true
})

const maskToken = token => token ? `${token.slice(0, 8)}...${token.slice(-6)}` : ''

const formatTime = t => {
  if (!t) return '永久'
  const d = new Date(t)
  return d.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

const isExpired = row => {
  if (!row.expires_at) return false
  return new Date(row.expires_at) < new Date()
}

const unitLabel = unit => {
  const map = { minutes: '分钟', hours: '小时', days: '天' }
  return map[unit] || unit
}

const rateLimitUnitLabel = unit => {
  const map = { seconds: '秒', minutes: '分钟', hours: '小时' }
  return map[unit] || '秒'
}

async function loadData() {
  loading.value = true
  try {
    const [keysRes, modelsRes] = await Promise.all([
      api.get('/api/temp-keys').catch(() => ({ data: [] })),
      api.get('/api/models').catch(() => ({ data: [] }))
    ])
    keys.value = keysRes.data || []
    modelOptions.value = (modelsRes.data || [])
      .filter(m => m.is_active)
      .map(m => ({
        label: m.display_name || m.original_id,
        value: m.display_name || m.original_id
      }))
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.name = ''
  form.allowed_models = []
  form.model_limits = {}
  form.max_requests = 0
  form.rate_limit_count = 0
  form.rate_limit_window = 0
  form.rate_limit_unit = 'seconds'
  form.concurrency_limit = 0
  form.expire_duration = 0
  form.expire_unit = ''
  form.is_active = true
}

function openCreate() {
  editingId.value = null
  resetForm()
  dialogVisible.value = true
}

function openEdit(row) {
  editingId.value = row.id
  form.name = row.name || ''
  form.allowed_models = [...(row.allowed_models || [])]
  form.model_limits = { ...(row.model_limits || {}) }
  form.max_requests = row.max_requests || 0
  form.rate_limit_count = row.rate_limit_count || 0
  form.rate_limit_window = row.rate_limit_window || 0
  form.rate_limit_unit = row.rate_limit_unit || 'seconds'
  form.concurrency_limit = row.concurrency_limit || 0
  form.expire_duration = row.expire_duration || 0
  form.expire_unit = row.expire_unit || ''
  form.is_active = row.is_active
  dialogVisible.value = true
}

function onModelsChange(models) {
  const limits = { ...form.model_limits }
  Object.keys(limits).forEach(key => {
    if (!models.includes(key)) delete limits[key]
  })
  form.model_limits = limits
}

async function submit() {
  // 校验
  if (!form.allowed_models.length) {
    ElMessage.error('请至少选择一个模型')
    return
  }

  if (form.expire_duration > 0 && !form.expire_unit) {
    ElMessage.error('请选择有效期单位')
    return
  }

  if (form.rate_limit_count > 0 && form.rate_limit_window <= 0) {
    ElMessage.error('请设置速率限制的时间窗口')
    return
  }

  saving.value = true
  try {
    const payload = {
      name: form.name,
      allowed_models: form.allowed_models,
      model_limits: form.model_limits,
      max_requests: form.max_requests,
      rate_limit_count: form.rate_limit_count,
      rate_limit_window: form.rate_limit_window,
      rate_limit_unit: form.rate_limit_unit,
      concurrency_limit: form.concurrency_limit,
      expire_duration: form.expire_duration,
      expire_unit: form.expire_unit,
      is_active: form.is_active
    }

    if (editingId.value) {
      await api.put(`/api/temp-keys/${editingId.value}`, payload)
      ElMessage.success('已更新')
    } else {
      await api.post('/api/temp-keys', payload)
      ElMessage.success('已创建')
    }
    dialogVisible.value = false
    loadData()
  } finally {
    saving.value = false
  }
}

async function remove(row) {
  await api.delete(`/api/temp-keys/${row.id}`)
  ElMessage.success('已删除')
  loadData()
}

function copy(text) {
  if (!text) return
  navigator.clipboard.writeText(text)
  ElMessage.success('已复制')
}

onMounted(loadData)
</script>

<style scoped>
.temp-keys {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tip {
  margin-bottom: 8px;
}
.token-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.token-text {
  font-family: monospace;
  font-size: 12px;
}
.tag-wrap {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.expire-duration {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.limits {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}
.limit-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.limit-name {
  min-width: 140px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.limit-hint {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.form-hint {
  margin-left: 8px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.rate-limit-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.expire-row {
  display: flex;
  align-items: center;
}
.el-divider {
  margin: 16px 0 8px;
}
</style>
