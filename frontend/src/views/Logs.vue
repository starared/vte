<template>
  <div class="logs">
    <div class="header">
      <h2>VTE 日志</h2>
      <div>
        <el-switch v-model="autoRefresh" active-text="自动刷新" style="margin-right: 12px" />
        <el-button @click="loadLogs" :loading="loading">刷新</el-button>
        <el-button type="danger" @click="clearLogs">清空</el-button>
      </div>
    </div>

    <!-- 统计卡片 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-value">{{ stats.total_requests }}</div>
        <div class="stat-label">总请求</div>
      </div>
      <div class="stat-card success">
        <div class="stat-value">{{ stats.success_requests }}</div>
        <div class="stat-label">成功</div>
      </div>
      <div class="stat-card error">
        <div class="stat-value">{{ stats.error_requests }}</div>
        <div class="stat-label">失败</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ successRate }}%</div>
        <div class="stat-label">成功率</div>
      </div>
      <el-button size="small" text @click="resetStats">重置统计</el-button>
    </div>

    <div class="terminal" ref="terminalRef">
      <div v-for="(line, idx) in logs" :key="idx" class="log-line" :class="getLogClass(line)">
        {{ line }}
      </div>
      <div v-if="logs.length === 0" class="empty">暂无日志</div>
    </div>

    <div class="tip">
      最多保留 100 条，自动清理旧日志
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const loading = ref(false)
const logs = ref([])
const stats = ref({ total_requests: 0, success_requests: 0, error_requests: 0 })
const autoRefresh = ref(true)
const terminalRef = ref(null)
let timer = null

const successRate = computed(() => {
  if (stats.value.total_requests === 0) return 0
  return Math.round((stats.value.success_requests / stats.value.total_requests) * 100)
})

async function loadLogs() {
  loading.value = true
  try {
    const [logsRes, statsRes] = await Promise.all([
      api.get('/api/logs'),
      api.get('/api/logs/stats')
    ])
    logs.value = logsRes.data.logs || []
    stats.value = statsRes.data
    nextTick(() => {
      if (terminalRef.value) {
        terminalRef.value.scrollTop = terminalRef.value.scrollHeight
      }
    })
  } finally {
    loading.value = false
  }
}

async function clearLogs() {
  await ElMessageBox.confirm('确定清空所有日志？', '确认')
  await api.delete('/api/logs')
  ElMessage.success('日志已清空')
  loadLogs()
}

async function resetStats() {
  await ElMessageBox.confirm('确定重置统计数据？', '确认')
  await api.delete('/api/logs/stats')
  ElMessage.success('统计已重置')
  loadLogs()
}

function getLogClass(line) {
  if (line.includes('[ERROR]')) return 'error'
  if (line.includes('[WARN]')) return 'warn'
  if (line.includes('[DEBUG]')) return 'debug'
  return 'info'
}

function startAutoRefresh() {
  if (timer) clearInterval(timer)
  timer = setInterval(loadLogs, 3000)
}

function stopAutoRefresh() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

watch(autoRefresh, (val) => {
  if (val) startAutoRefresh()
  else stopAutoRefresh()
})

onMounted(() => {
  loadLogs()
  if (autoRefresh.value) startAutoRefresh()
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
.terminal {
  background: #1e1e1e;
  color: #d4d4d4;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  padding: 16px;
  border-radius: 8px;
  height: 500px;
  overflow-y: auto;
  line-height: 1.6;
}
.log-line {
  white-space: pre-wrap;
  word-break: break-all;
}
.log-line.error { color: #f56c6c; }
.log-line.warn { color: #e6a23c; }
.log-line.debug { color: #909399; }
.log-line.info { color: #67c23a; }
.empty {
  color: #606266;
  text-align: center;
  padding: 40px;
}
.tip {
  margin-top: 12px;
  color: #909399;
  font-size: 13px;
}
.stats-row {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  align-items: center;
  flex-wrap: wrap;
}
.stat-card {
  background: #fff;
  border-radius: 8px;
  padding: 12px 20px;
  text-align: center;
  min-width: 70px;
  box-shadow: 0 1px 3px rgba(0,0,0,0.1);
}
.stat-card.success .stat-value { color: #67c23a; }
.stat-card.error .stat-value { color: #f56c6c; }
.stat-value {
  font-size: 24px;
  font-weight: 600;
  color: #303133;
}
.stat-label {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

@media (max-width: 768px) {
  .header h2 { font-size: 18px; }
  .terminal { height: 350px; font-size: 11px; padding: 12px; }
  .stat-card { padding: 8px 12px; min-width: 60px; }
  .stat-value { font-size: 18px; }
}
</style>
