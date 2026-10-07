<template>
  <div class="api-key-field">
    <el-input :model-value="visible ? value : masked" readonly class="key-input" />
    <div class="key-actions">
      <el-button @click="visible = !visible">{{ visible ? '隐藏' : '显示' }}</el-button>
      <el-button @click="copyText(value)">复制</el-button>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { copyText } from '../utils'

defineProps({
  value: { type: String, default: '' }
})

const visible = ref(false)
const masked = '••••••••••••••••••••••••••••••••'
</script>

<style scoped>
.api-key-field {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  max-width: 560px;
}
.key-input {
  flex: 1;
  min-width: 0;
}
.key-actions {
  display: flex;
  gap: 8px;
}
.key-actions .el-button + .el-button {
  margin-left: 0;
}

@media (max-width: 768px) {
  .api-key-field {
    flex-direction: column;
    align-items: stretch;
    max-width: none;
  }
  .key-actions .el-button {
    flex: 1;
  }
}
</style>
