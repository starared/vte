<template>
  <div class="login-container">
    <el-card class="login-card">
      <h2>VTE</h2>
      <p class="subtitle">多后端 LLM API 网关</p>
      <el-form @submit.prevent="handleLogin" :model="form">
        <el-form-item>
          <el-input v-model="form.username" placeholder="用户名" prefix-icon="User" size="large" />
        </el-form-item>
        <el-form-item>
          <el-input v-model="form.password" type="password" placeholder="密码" prefix-icon="Lock" size="large" show-password />
        </el-form-item>
        <el-button type="primary" native-type="submit" :loading="loading" size="large" style="width: 100%">
          登录
        </el-button>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'
import { ElMessage } from 'element-plus'

const router = useRouter()
const userStore = useUserStore()
const loading = ref(false)
const form = ref({ username: '', password: '' })

async function handleLogin() {
  if (!form.value.username || !form.value.password) {
    ElMessage.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    await userStore.login(form.value.username, form.value.password)
    ElMessage.success('登录成功')
    router.push('/')
  } catch {
    // 错误已在拦截器处理
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #34d399 0%, #0d9488 55%, #0f766e 100%);
  padding: 20px;
}
.login-card {
  width: 400px;
  max-width: 100%;
  padding: 28px 24px;
  border-radius: 20px;
  border: none;
  box-shadow: 0 24px 60px rgba(6, 78, 59, 0.28);
}
.login-card h2 {
  text-align: center;
  margin-bottom: 6px;
  font-size: 30px;
  font-weight: 800;
  letter-spacing: 3px;
  background: linear-gradient(135deg, #10b981, #0d9488);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}
.subtitle {
  text-align: center;
  color: #909399;
  margin-bottom: 28px;
  font-size: 14px;
}

@media (max-width: 480px) {
  .login-card { padding: 20px 16px; }
}
</style>
