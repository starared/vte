<template>
  <div class="token-stats">
    <div class="header">
      <h2>Token 消耗统计</h2>
      <div>
        <el-button @click="loadStats" :loading="loading">刷新</el-button>
        <el-button type="danger" @click="resetStats">重置今日统计</el-button>
      </div>
    </div>

    <!-- 总览卡片 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-value">{{ formatNumber(stats.total_tokens) }}</div>
        <div class="stat-label">今日总Token</div>
      </div>
      <div class="stat-card prompt">
        <div class="stat-value">{{ formatNumber(stats.prompt_tokens) }}</div>
        <div class="stat-label">今日输入Token</div>
      </div>
      <div class="stat-card completion">
        <div class="stat-value">{{ formatNumber(stats.completion_tokens) }}</div>
        <div class="stat-label">今日输出Token</div>
      </div>
    </div>

    <!-- 模型使用统计 -->
    <el-card class="table-card">
      <template #header>
        <span>模型使用详情（今日）</span>
      </template>
      <el-table :data="stats.model_stats" stripe>
        <el-table-column prop="model_name" label="模型名称" min-width="150" />
        <el-table-column prop="provider_name" label="提供商" width="120" />
        <el-table-column prop="request_count" label="请求次数" width="100" align="right" />
        <el-table-column prop="total_tokens" label="总Token" width="120" align="right">
          <template #default="{ row }">
            {{ formatNumber(row.total_tokens) }}
          </template>
        </el-table-column>
        <el-table-column prop="prompt_tokens" label="输入Token" width="120" align="right">
          <template #default="{ row }">
            {{ formatNumber(row.prompt_tokens) }}
          </template>
        </el-table-column>
        <el-table-column prop="completion_tokens" label="输出Token" width="120" align="right">
          <template #default="{ row }">
            {{ formatNumber(row.completion_tokens) }}
          </template>
        </el-table-column>
      </el-table>
      <div v-if="!stats.model_stats || stats.model_stats.length === 0" class="empty">
        暂无数据
      </div>
    </el-card>

    <div class="tip">
      统计周期：每天 15:00 至 次日 15:00（北京时间 UTC+8），到期自动重置
      <span class="tip-sep">·</span>
      服务器时间：{{ stats.server_time || '--' }}
      <span class="tip-sep">·</span>
      下次重置：{{ stats.next_reset_time || '--' }}
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const loading = ref(false)
let timer = null

const stats = ref({
  total_tokens: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  hourly_stats: [],
  model_stats: [],
  server_time: '',
  next_reset_time: '',
  timezone: ''
})

function formatNumber(num) {
  if (!num) return '0'
  return num.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',')
}

async function loadStats(showLoading = true) {
  if (showLoading) loading.value = true
  try {
    const res = await api.get('/api/tokens/stats')
    stats.value = res.data
  } catch (error) {
    console.error('加载统计失败:', error)
  } finally {
    if (showLoading) loading.value = false
  }
}

async function resetStats() {
  await ElMessageBox.confirm('确定重置今日统计数据？', '确认')
  await api.delete('/api/tokens/stats')
  ElMessage.success('统计已重置')
  loadStats()
}

function startAutoRefresh() {
  if (timer) clearInterval(timer)
  timer = setInterval(() => loadStats(false), 10000) // 每10秒刷新
}

function stopAutoRefresh() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

onMounted(() => {
  loadStats()
  startAutoRefresh()
})

onUnmounted(() => {
  stopAutoRefresh()
})
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

.stats-row {
  display: flex;
  gap: 16px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}

.stat-card {
  flex: 1;
  min-width: 160px;
  position: relative;
  overflow: hidden;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 16px;
  padding: 22px 24px;
  box-shadow: 0 1px 2px rgba(41, 37, 36, 0.04), 0 8px 24px rgba(41, 37, 36, 0.05);
}
/* 顶部彩条：区分总量 / 输入 / 输出 */
.stat-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 4px;
  background: var(--vte-accent-gradient);
}
.stat-card.prompt::before { background: linear-gradient(135deg, #38bdf8, #0ea5e9); }
.stat-card.completion::before { background: linear-gradient(135deg, #fbbf24, #f59e0b); }

.stat-value {
  font-size: 32px;
  font-weight: 700;
  color: var(--el-text-color-primary);
  margin-bottom: 6px;
  line-height: 1.1;
}
.stat-card.prompt .stat-value { color: #0ea5e9; }
.stat-card.completion .stat-value { color: #f59e0b; }

.stat-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.table-card {
  margin-bottom: 20px;
}

.empty {
  color: var(--el-text-color-secondary);
  text-align: center;
  padding: 40px;
}

.tip {
  margin-top: 12px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  text-align: center;
}
.tip-sep { margin: 0 8px; opacity: 0.5; }

@media (max-width: 768px) {
  .header h2 { font-size: 18px; }
  .stat-card { padding: 16px; min-width: 120px; }
  .stat-value { font-size: 24px; }
  .tip-sep { display: none; }
  .tip { line-height: 1.9; }
}
</style>
