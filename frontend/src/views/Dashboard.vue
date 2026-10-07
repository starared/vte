<template>
  <div class="dashboard">
    <h2>仪表盘</h2>
    
    <el-row :gutter="20" class="stats">
      <el-col :span="8">
        <el-card shadow="hover">
          <el-statistic title="提供商数量" :value="stats.providers" />
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="hover">
          <el-statistic title="已启用模型" :value="stats.activeModels" />
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="hover">
          <el-statistic title="总模型数" :value="stats.totalModels" />
        </el-card>
      </el-col>
    </el-row>

    <el-card class="api-info">
      <template #header>
        <span>API 接入信息</span>
      </template>
      <el-descriptions :column="1" border :direction="isMobile ? 'vertical' : 'horizontal'">
        <el-descriptions-item label="API 地址">
          <el-input :model-value="apiUrl + '/v1'" readonly class="url-input">
            <template #append>
              <el-button @click="copyText(apiUrl + '/v1')">复制</el-button>
            </template>
          </el-input>
        </el-descriptions-item>
        <el-descriptions-item label="API Key">
          <ApiKeyField :value="userStore.user?.api_key" />
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
import { useIsMobile } from '../composables/useIsMobile'
import { copyText } from '../utils'
import ApiKeyField from '../components/ApiKeyField.vue'
import api from '../api'

const userStore = useUserStore()
const isMobile = useIsMobile()
const stats = ref({ providers: 0, activeModels: 0, totalModels: 0 })

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

onMounted(loadStats)
</script>

<style scoped>
.dashboard h2 { margin-bottom: 20px; }
.stats { margin-bottom: 20px; }
.api-info { margin-bottom: 20px; }
.url-input { max-width: 560px; }
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
  .stats { margin-left: -4px !important; margin-right: -4px !important; }
  .stats .el-col { padding-left: 4px !important; padding-right: 4px !important; }
  .stats :deep(.el-card__body) { padding: 12px; }
  .stats :deep(.el-statistic__head) { font-size: 12px; }
  .stats :deep(.el-statistic__content) { font-size: 22px; }
  .url-input { max-width: none; }
  .code { font-size: 11px; padding: 12px; }
}
</style>
