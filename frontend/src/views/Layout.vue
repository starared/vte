<template>
  <el-container class="layout">
    <!-- 移动端遮罩 -->
    <div class="mobile-overlay" v-if="sidebarOpen" @click="sidebarOpen = false"></div>
    
    <el-aside :width="sidebarWidth" :class="{ 'mobile-open': sidebarOpen }">
      <div class="logo">VTE</div>
      <el-menu :default-active="route.path" router :background-color="menuBgColor" text-color="#bfcbd9" active-text-color="#409EFF" @select="handleMenuSelect">
        <el-menu-item index="/dashboard">
          <el-icon><DataAnalysis /></el-icon>
          <span>仪表盘</span>
        </el-menu-item>
        <el-menu-item index="/providers">
          <el-icon><Connection /></el-icon>
          <span>提供商</span>
        </el-menu-item>
        <el-menu-item index="/models">
          <el-icon><Cpu /></el-icon>
          <span>模型管理</span>
        </el-menu-item>
        <el-menu-item index="/logs">
          <el-icon><Document /></el-icon>
          <span>请求日志</span>
        </el-menu-item>
        <el-menu-item index="/token-stats">
          <el-icon><TrendCharts /></el-icon>
          <span>Token统计</span>
        </el-menu-item>
        <el-menu-item index="/settings">
          <el-icon><Setting /></el-icon>
          <span>设置</span>
        </el-menu-item>
        <el-menu-item index="/about">
          <el-icon><InfoFilled /></el-icon>
          <span>关于</span>
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header>
        <el-icon class="menu-toggle" @click="sidebarOpen = !sidebarOpen"><Fold /></el-icon>
        <div class="header-right">
          <el-tooltip :content="themeTooltip" placement="bottom">
            <el-button text circle @click="themeStore.toggleTheme">
              <el-icon :size="18">
                <Sunny v-if="themeStore.theme === 'light'" />
                <Moon v-else-if="themeStore.theme === 'dark'" />
                <Monitor v-else />
              </el-icon>
            </el-button>
          </el-tooltip>
          <span class="username">{{ userStore.user?.username }}</span>
          <el-button text @click="handleLogout">退出</el-button>
        </div>
      </el-header>
      <el-main>
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'
import { useThemeStore } from '../stores/theme'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const themeStore = useThemeStore()
const sidebarOpen = ref(false)
const isMobile = ref(false)

const sidebarWidth = computed(() => isMobile.value ? '200px' : '200px')

const menuBgColor = computed(() => {
  return themeStore.theme === 'dark' || 
    (themeStore.theme === 'auto' && window.matchMedia('(prefers-color-scheme: dark)').matches)
    ? '#1d1e1f' : '#304156'
})

const themeTooltip = computed(() => {
  const labels = { light: '亮色模式', dark: '暗色模式', auto: '跟随系统' }
  return labels[themeStore.theme]
})

function checkMobile() {
  isMobile.value = window.innerWidth < 768
  if (!isMobile.value) sidebarOpen.value = false
}

function handleMenuSelect() {
  if (isMobile.value) sidebarOpen.value = false
}

function handleLogout() {
  userStore.logout()
  router.push('/login')
}

onMounted(() => {
  checkMobile()
  window.addEventListener('resize', checkMobile)
  themeStore.loadTheme()
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
})
</script>

<style scoped>
.layout {
  min-height: 100vh;
}
.el-aside {
  background: var(--vte-sidebar-bg);
  transition: transform 0.3s, background-color 0.3s;
}
.logo {
  height: 60px;
  line-height: 60px;
  text-align: center;
  color: #fff;
  font-size: 18px;
  font-weight: bold;
}
.el-header {
  background: var(--vte-header-bg);
  display: flex;
  align-items: center;
  justify-content: space-between;
  box-shadow: 0 1px 4px rgba(0,0,0,0.08);
  padding: 0 16px;
  transition: background-color 0.3s;
}
.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}
.el-main {
  background: var(--el-bg-color-page);
  padding: 20px;
  transition: background-color 0.3s;
}
.menu-toggle {
  display: none;
  font-size: 22px;
  cursor: pointer;
}
.mobile-overlay {
  display: none;
}

/* 移动端适配 */
@media (max-width: 768px) {
  .el-aside {
    position: fixed;
    left: 0;
    top: 0;
    bottom: 0;
    z-index: 1000;
    transform: translateX(-100%);
  }
  .el-aside.mobile-open {
    transform: translateX(0);
  }
  .menu-toggle {
    display: block;
  }
  .mobile-overlay {
    display: block;
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0,0,0,0.5);
    z-index: 999;
  }
  .el-main {
    padding: 12px;
  }
  .username {
    display: none;
  }
  .header-right {
    gap: 4px;
  }
}
</style>
