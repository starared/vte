import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import App from './App.vue'
import router from './router'
import './styles/theme.css'

// 图标在各组件中按需 import，不再全局注册全部 290+ 个图标

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(ElementPlus)
app.mount('#app')
