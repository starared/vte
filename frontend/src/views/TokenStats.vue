<template>
  <div class="token-stats">
    <div class="header">
      <h2>Token 消耗统计</h2>
      <div>
        <el-switch v-model="autoRefresh" active-text="自动刷新" style="margin-right: 12px" />
        <el-button @click="loadStats" :loading="loading">刷新</el-button>
        <el-button type="danger" @click="resetStats">重置今日统计</el-button>
      </div>
    </div>

    <!-- 总览卡片 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-value">{{ formatNumber(stats.total_tokens) }}</div>
        <div class="stat-label">总Token</div>
      </div>
      <div class="stat-card prompt">
        <div class="stat-value">{{ formatNumber(stats.prompt_tokens) }}</div>
        <div class="stat-label">输入Token</div>
      </div>
      <div class="stat-card completion">
        <div class="stat-value">{{ formatNumber(stats.completion_tokens) }}</div>
        <div class="stat-label">输出Token</div>
      </div>
    </div>

    <!-- 24小时趋势图 -->
    <el-card class="chart-card">
      <template #header>
        <div class="card-header">
          <span>24小时Token消耗趋势</span>
          <span class="subtitle">每天下午3点自动刷新</span>
        </div>
      </template>
      <div ref="hourlyChartRef" style="height: 300px"></div>
    </el-card>

    <!-- 模型使用统计 -->
    <el-card class="table-card">
      <template #header>
        <span>模型使用详情</span>
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
      统计数据每天下午3点自动刷新，历史记录保留30天
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import * as echarts from 'echarts'
import api from '../api'

const loading = ref(false)
const autoRefresh = ref(true)
const hourlyChartRef = ref(null)
let hourlyChart = null
let timer = null

const stats = ref({
  total_tokens: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  hourly_stats: [],
  model_stats: []
})

function formatNumber(num) {
  if (!num) return '0'
  return num.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',')
}

async function loadStats() {
  loading.value = true
  try {
    const res = await api.get('/api/tokens/stats')
    stats.value = res.data
    nextTick(() => {
      renderHourlyChart()
    })
  } catch (error) {
    console.error('加载统计失败:', error)
  } finally {
    loading.value = false
  }
}

async function resetStats() {
  await ElMessageBox.confirm('确定重置今日统计数据？', '确认')
  await api.delete('/api/tokens/stats')
  ElMessage.success('统计已重置')
  loadStats()
}

function renderHourlyChart() {
  if (!hourlyChartRef.value) return
  
  if (!hourlyChart) {
    hourlyChart = echarts.init(hourlyChartRef.value)
  }

  // 重新排序：从15:00开始到第二天14:00
  const hourlyData = stats.value.hourly_stats || []
  const reorderedData = []
  
  // 从15点到23点
  for (let i = 15; i < 24; i++) {
    const data = hourlyData.find(h => h.hour === i) || { hour: i, total_tokens: 0, request_count: 0 }
    reorderedData.push(data)
  }
  // 从0点到14点
  for (let i = 0; i < 15; i++) {
    const data = hourlyData.find(h => h.hour === i) || { hour: i, total_tokens: 0, request_count: 0 }
    reorderedData.push(data)
  }

  const hours = reorderedData.map(h => `${h.hour}:00`)
  const tokens = reorderedData.map(h => h.total_tokens)
  const requests = reorderedData.map(h => h.request_count)

  const option = {
    tooltip: {
      trigger: 'axis',
      formatter: function(params) {
        let result = params[0].axisValue + '<br/>'
        params.forEach(item => {
          result += item.marker + item.seriesName + ': ' + item.value + '<br/>'
        })
        return result
      }
    },
    legend: {
      data: ['Token数量', '请求次数'],
      top: 10
    },
    grid: {
      left: '3%',
      right: '5%',
      bottom: '3%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      data: hours,
      boundaryGap: false
    },
    yAxis: [
      {
        type: 'value',
        name: 'Token数量',
        position: 'left',
        axisLine: {
          lineStyle: {
            color: '#409EFF'
          }
        }
      },
      {
        type: 'value',
        name: '请求次数',
        position: 'right',
        axisLine: {
          lineStyle: {
            color: '#67C23A'
          }
        }
      }
    ],
    series: [
      {
        name: 'Token数量',
        type: 'line',
        smooth: true,
        yAxisIndex: 0,
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(64, 158, 255, 0.3)' },
              { offset: 1, color: 'rgba(64, 158, 255, 0.05)' }
            ]
          }
        },
        lineStyle: {
          color: '#409EFF',
          width: 2
        },
        itemStyle: {
          color: '#409EFF'
        },
        data: tokens
      },
      {
        name: '请求次数',
        type: 'line',
        smooth: true,
        yAxisIndex: 1,
        lineStyle: {
          color: '#67C23A',
          width: 2
        },
        itemStyle: {
          color: '#67C23A'
        },
        data: requests
      }
    ]
  }

  hourlyChart.setOption(option)
}

function startAutoRefresh() {
  if (timer) clearInterval(timer)
  timer = setInterval(loadStats, 30000) // 每30秒刷新
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
  loadStats()
  if (autoRefresh.value) startAutoRefresh()
  
  // 监听窗口大小变化
  window.addEventListener('resize', () => {
    if (hourlyChart) hourlyChart.resize()
  })
})

onUnmounted(() => {
  stopAutoRefresh()
  if (hourlyChart) {
    hourlyChart.dispose()
    hourlyChart = null
  }
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
  min-width: 150px;
  background: #fff;
  border-radius: 8px;
  padding: 20px;
  text-align: center;
  box-shadow: 0 2px 12px rgba(0,0,0,0.08);
}

.stat-card.prompt .stat-value { color: #409EFF; }
.stat-card.completion .stat-value { color: #67C23A; }

.stat-value {
  font-size: 32px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 8px;
}

.stat-label {
  font-size: 14px;
  color: #909399;
}

.chart-card, .table-card {
  margin-bottom: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.subtitle {
  font-size: 12px;
  color: #909399;
  font-weight: normal;
}

.empty {
  color: #909399;
  text-align: center;
  padding: 40px;
}

.tip {
  margin-top: 12px;
  color: #909399;
  font-size: 13px;
  text-align: center;
}

@media (max-width: 768px) {
  .header h2 { font-size: 18px; }
  .stat-card { padding: 12px; min-width: 120px; }
  .stat-value { font-size: 24px; }
}
</style>
