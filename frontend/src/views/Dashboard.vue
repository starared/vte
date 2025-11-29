<template>
  <div class="dashboard">
    <h2>仪表盘</h2>
    
    <el-row :gutter="20" class="stats">
      <el-col :xs="24" :sm="8">
        <el-card shadow="hover">
          <el-statistic title="提供商数量" :value="stats.providers" />
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="8">
        <el-card shadow="hover">
          <el-statistic title="已启用模型" :value="stats.activeModels" />
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="8">
        <el-card shadow="hover">
          <el-statistic title="总模型数" :value="stats.totalModels" />
        </el-card>
      </el-col>
    </el-row>

    <el-card class="api-info">
      <template #header>
        <span>API 接入信息</span>
      </template>
      <el-descriptions :column="1" border>
        <el-descriptions-item label="API 地址">
          <el-input :value="apiUrl + '/v1'" readonly>
            <template #append>
              <el-button @click="copy(apiUrl + '/v1')">复制</el-button>
            </template>
          </el-input>
        </el-descriptions-item>
        <el-descriptions-item label="API Key">
          <div class="api-key-row">
            <el-input :value="showApiKey ? userStore.user?.api_key : '••••••••••••••••••••••••••••••••'" readonly style="flex: 1" />
            <el-button @click="showApiKey = !showApiKey" style="margin-left: 8px">{{ showApiKey ? '隐藏' : '显示' }}</el-button>
            <el-button @click="copy(userStore.user?.api_key)" style="margin-left: 8px">复制</el-button>
          </div>
        </el-descriptions-item>
      </el-descriptions>
      <div class="tip">
        <p>在支持 OpenAI API 的客户端中配置以上地址和 Key 即可使用</p>
      </div>
    </el-card>

    <el-card class="quick-test">
      <template #header>
        <span>快速测试</span>
      </template>
      <pre class="code">curl {{ apiUrl + '/v1' }}/chat/completions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "模型名称",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'</pre>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { useUserStore } from '../stores/user'
import { ElMessage } from 'element-plus'
import api from '../api'

const userStore = useUserStore()
const stats = ref({ providers: 0, activeModels: 0, totalModels: 0 })
const showApiKey = ref(false)

const apiUrl = computed(() => window.location.origin)

async function loadStats() {
  try {
    const [providersRes, modelsRes] = await Promise.all([
      api.get('/api/providers'),
      api.get('/api/models')
    ])
    stats.value.providers = providersRes.data.length
    stats.value.totalModels = modelsRes.data.length
    stats.value.activeModels = modelsRes.data.filter(m => m.is_active).length
  } catch {}
}

function copy(text) {
  navigator.clipboard.writeText(text)
  ElMessage.success('已复制')
}

onMounted(loadStats)
</script>

<style scoped>
.dashboard h2 { margin-bottom: 20px; }
.stats { margin-bottom: 20px; }
.api-info { margin-bottom: 20px; }
.api-key-row { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.tip { margin-top: 16px; color: var(--el-text-color-secondary); font-size: 14px; }
.code {
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
  padding: 16px;
  border-radius: 4px;
  overflow-x: auto;
  font-size: 13px;
}

@media (max-width: 768px) {
  .stats .el-col { margin-bottom: 12px; }
  .api-key-row { flex-direction: column; align-items: stretch; }
  .api-key-row .el-input { width: 100% !important; }
  .api-key-row .el-button { margin-left: 0 !important; margin-top: 8px; }
  .code { font-size: 11px; padding: 12px; }
}
</style>
