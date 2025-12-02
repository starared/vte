<template>
  <div class="models">
    <div class="header">
      <h2>模型管理</h2>
      <div>
        <el-button @click="batchToggle(true)" :disabled="!selectedIds.length">批量启用</el-button>
        <el-button @click="batchToggle(false)" :disabled="!selectedIds.length">批量禁用</el-button>
      </div>
    </div>

    <!-- 搜索和筛选 -->
    <div class="filter">
      <el-input v-model="search" placeholder="搜索模型..." clearable style="width: 300px" />
      <el-select v-model="filterProvider" placeholder="筛选提供商" clearable style="width: 150px; margin-left: 12px">
        <el-option v-for="p in providerOptions" :key="p" :label="p" :value="p" />
      </el-select>
      <el-select v-model="filterStatus" placeholder="筛选状态" clearable style="width: 120px; margin-left: 12px">
        <el-option label="已启用" :value="true" />
        <el-option label="已禁用" :value="false" />
      </el-select>
    </div>

    <el-table :data="pagedModels" v-loading="loading" stripe @selection-change="handleSelect">
      <el-table-column type="selection" width="50" />
      <el-table-column prop="provider_name" label="提供商" width="120" />
      <el-table-column prop="original_id" label="原始模型 ID" min-width="200" />
      <el-table-column prop="display_name" label="显示名称" min-width="200">
        <template #default="{ row }">
          <div class="display-name-cell">
            <span>{{ row.display_name || row.original_id }}</span>
            <el-tag v-if="row.custom_name" size="small" type="warning" class="custom-tag">自定义</el-tag>
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="is_active" label="状态" width="80">
        <template #default="{ row }">
          <el-switch v-model="row.is_active" @change="updateModelStatus(row)" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="180">
        <template #default="{ row }">
          <el-button size="small" type="primary" text @click="openEditDialog(row)">编辑名称</el-button>
          <el-button v-if="row.custom_name" size="small" type="warning" text @click="resetDisplayName(row)">重置</el-button>
          <el-button size="small" type="danger" text @click="deleteModel(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 分页 -->
    <div class="pagination">
      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        :page-sizes="[10, 20, 50, 100]"
        :total="filteredModels.length"
        layout="total, sizes, prev, pager, next"
        @size-change="currentPage = 1"
      />
    </div>

    <!-- 编辑显示名称对话框 -->
    <el-dialog v-model="editDialogVisible" title="编辑显示名称" width="500px">
      <el-form label-width="100px">
        <el-form-item label="原始模型ID">
          <el-input :model-value="editingModel?.original_id" disabled />
        </el-form-item>
        <el-form-item label="显示名称">
          <el-input v-model="editDisplayName" placeholder="输入自定义显示名称" />
        </el-form-item>
        <el-form-item>
          <el-text type="info" size="small">
            提示：用户通过 API 请求时使用此显示名称，实际转发时使用原始模型 ID
          </el-text>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveDisplayName" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onActivated } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const loading = ref(false)
const models = ref([])
const selectedIds = ref([])
const search = ref('')
const filterProvider = ref('')
const filterStatus = ref(null)
const currentPage = ref(1)
const pageSize = ref(20)

// 编辑对话框
const editDialogVisible = ref(false)
const editingModel = ref(null)
const editDisplayName = ref('')
const saving = ref(false)

const providerOptions = computed(() => {
  const set = new Set(models.value.map(m => m.provider_name))
  return Array.from(set).filter(Boolean)
})

const filteredModels = computed(() => {
  return models.value.filter(m => {
    const keyword = search.value.toLowerCase()
    if (keyword && !m.original_id.toLowerCase().includes(keyword) && !m.display_name?.toLowerCase().includes(keyword)) return false
    if (filterProvider.value && m.provider_name !== filterProvider.value) return false
    if (filterStatus.value !== null && m.is_active !== filterStatus.value) return false
    return true
  })
})

const pagedModels = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return filteredModels.value.slice(start, start + pageSize.value)
})

async function loadModels() {
  loading.value = true
  try {
    const res = await api.get('/api/models')
    models.value = res.data
  } finally {
    loading.value = false
  }
}

function handleSelect(rows) {
  selectedIds.value = rows.map(r => r.id)
}

async function updateModelStatus(row) {
  await api.put(`/api/models/${row.id}`, {
    is_active: row.is_active
  })
}

function openEditDialog(row) {
  editingModel.value = row
  editDisplayName.value = row.display_name || row.original_id
  editDialogVisible.value = true
}

async function saveDisplayName() {
  if (!editDisplayName.value.trim()) {
    ElMessage.warning('显示名称不能为空')
    return
  }
  saving.value = true
  try {
    await api.put(`/api/models/${editingModel.value.id}`, {
      display_name: editDisplayName.value.trim()
    })
    ElMessage.success('保存成功')
    editDialogVisible.value = false
    loadModels()
  } finally {
    saving.value = false
  }
}

async function resetDisplayName(row) {
  await ElMessageBox.confirm('确定重置为自动生成的名称？', '确认')
  try {
    const res = await api.post(`/api/models/${row.id}/reset-name`)
    ElMessage.success('重置成功')
    loadModels()
  } catch (e) {
    ElMessage.error('重置失败')
  }
}

async function deleteModel(row) {
  await ElMessageBox.confirm('确定删除该模型？', '确认')
  await api.delete(`/api/models/${row.id}`)
  ElMessage.success('删除成功')
  loadModels()
}

async function batchToggle(active) {
  await api.post('/api/models/batch-toggle', {
    model_ids: selectedIds.value,
    is_active: active
  })
  ElMessage.success('操作成功')
  loadModels()
}

onMounted(loadModels)
onActivated(loadModels)  // 页面激活时自动刷新
</script>

<style scoped>
.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  flex-wrap: wrap;
  gap: 12px;
}
.filter {
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}
.pagination {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}
.display-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}
.custom-tag {
  flex-shrink: 0;
}

@media (max-width: 768px) {
  .header h2 { font-size: 18px; }
  .filter .el-input, .filter .el-select { 
    width: 100% !important; 
    margin-left: 0 !important; 
  }
  .pagination { justify-content: center; }
  .pagination :deep(.el-pagination) {
    flex-wrap: wrap;
    justify-content: center;
  }
}
</style>
