import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://127.0.0.1:8050',
      '/v1': 'http://127.0.0.1:8050'
    }
  }
})
