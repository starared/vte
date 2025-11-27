<template>
  <div class="about">
    <h2>关于 VTE</h2>

    <el-card class="section">
      <template #header>版本信息</template>
      <div class="version-info">
        <div class="version-row">
          <span class="label">当前版本：</span>
          <span class="value">v{{ currentVersion }}</span>
        </div>
        <div class="version-row">
          <span class="label">最新版本：</span>
          <span class="value" v-if="loading">检查中...</span>
          <span class="value" v-else-if="latestVersion">
            v{{ latestVersion }}
            <el-tag v-if="hasUpdate" type="warning" size="small" style="margin-left: 8px">有更新</el-tag>
            <el-tag v-else type="success" size="small" style="margin-left: 8px">已是最新</el-tag>
          </span>
          <span class="value error" v-else>检查失败</span>
        </div>
        <div class="version-actions">
          <el-button @click="checkUpdate" :loading="loading">检查更新</el-button>
          <el-button type="primary" @click="showUpdateGuide" :disabled="!hasUpdate">更新指南</el-button>
        </div>
      </div>
    </el-card>

    <el-card class="section">
      <template #header>更新方法</template>
      <div class="update-guide">
        <p><strong>Docker Compose 用户：</strong></p>
        <pre class="code"># 拉取最新镜像并重启
docker-compose pull && docker-compose up -d</pre>

        <p style="margin-top: 16px"><strong>Docker 命令行用户：</strong></p>
        <pre class="code"># 拉取最新镜像
docker pull rtyedfty/vte:latest

# 停止并删除旧容器
docker stop vte && docker rm vte

# 启动新容器（数据会保留）
docker run -d --name vte -p 8050:8050 -v vte-data:/app/data --restart unless-stopped rtyedfty/vte:latest</pre>
        
        <p style="margin-top: 16px"><strong>本地部署用户：</strong></p>
        <pre class="code"># 拉取最新代码
git pull

# 运行启动脚本（会自动重新构建）
start.bat      # Windows
./start.sh     # Linux/Mac</pre>
      </div>
    </el-card>

    <el-card class="section warning-card">
      <template #header>
        <span class="warning-header">⚠️ 使用声明</span>
      </template>
      <div class="warning-content">
        <p><strong>本项目仅供个人学习和研究使用，严禁用于任何商业用途。</strong></p>
        <ul>
          <li>禁止将本项目用于商业服务或盈利目的</li>
          <li>禁止出售、转售或以任何形式进行商业化运营</li>
          <li>使用本项目产生的任何法律责任由使用者自行承担</li>
        </ul>
        <p class="disclaimer">如有违反，后果自负。</p>
      </div>
    </el-card>

    <el-card class="section">
      <template #header>项目信息</template>
      <el-descriptions :column="1" border>
        <el-descriptions-item label="项目名称">VTE - Multi-backend LLM API Gateway</el-descriptions-item>
        <el-descriptions-item label="项目描述">轻量级多后端 LLM API 网关，统一管理多个 AI 服务提供商</el-descriptions-item>
        <el-descriptions-item label="GitHub">
          <a href="https://github.com/starared/vte" target="_blank">starared/vte</a>
        </el-descriptions-item>
        <el-descriptions-item label="Docker Hub">
          <a href="https://hub.docker.com/r/rtyedfty/vte" target="_blank">rtyedfty/vte</a>
        </el-descriptions-item>
      </el-descriptions>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const currentVersion = ref('1.0.1')
const latestVersion = ref('')
const loading = ref(false)

// 比较版本号大小
function compareVersion(v1, v2) {
  const parts1 = v1.split('.').map(Number)
  const parts2 = v2.split('.').map(Number)
  
  for (let i = 0; i < Math.max(parts1.length, parts2.length); i++) {
    const num1 = parts1[i] || 0
    const num2 = parts2[i] || 0
    
    if (num1 < num2) return -1
    if (num1 > num2) return 1
  }
  
  return 0
}

const hasUpdate = computed(() => {
  if (!latestVersion.value) return false
  // 只有当最新版本大于当前版本时才显示有更新
  return compareVersion(latestVersion.value, currentVersion.value) > 0
})

async function checkUpdate() {
  loading.value = true
  try {
    const res = await api.get('/api/version/check')
    currentVersion.value = res.data.current
    latestVersion.value = res.data.latest
    if (hasUpdate.value) {
      ElMessage.warning('发现新版本！')
    } else {
      ElMessage.success('已是最新版本')
    }
  } catch {
    ElMessage.error('检查更新失败')
  } finally {
    loading.value = false
  }
}

function showUpdateGuide() {
  ElMessageBox.alert(
    `请按照页面上的更新方法进行更新。\n\n当前版本: v${currentVersion.value}\n最新版本: v${latestVersion.value}`,
    '更新指南',
    { confirmButtonText: '知道了' }
  )
}

onMounted(() => {
  checkUpdate()
})
</script>


<style scoped>
.about h2 { margin-bottom: 20px; }
.section { margin-bottom: 20px; }
.version-info { line-height: 2; }
.version-row { display: flex; align-items: center; }
.version-row .label { width: 100px; color: #606266; }
.version-row .value { font-weight: 500; }
.version-row .value.error { color: #f56c6c; }
.version-actions { margin-top: 16px; }
.code {
  background: #f5f7fa;
  padding: 12px;
  border-radius: 4px;
  overflow-x: auto;
  font-size: 13px;
  font-family: 'Consolas', 'Monaco', monospace;
}
.warning-card :deep(.el-card__header) {
  background: #fdf6ec;
}
.warning-header { color: #e6a23c; font-weight: 600; }
.warning-content {
  color: #606266;
  line-height: 1.8;
}
.warning-content ul {
  margin: 12px 0;
  padding-left: 20px;
}
.warning-content li { margin: 8px 0; }
.disclaimer {
  color: #f56c6c;
  font-weight: 500;
  margin-top: 12px;
}
.update-guide p { margin-bottom: 8px; color: #303133; }

@media (max-width: 768px) {
  .about h2 { font-size: 18px; }
  .code { font-size: 11px; padding: 10px; }
}
</style>
