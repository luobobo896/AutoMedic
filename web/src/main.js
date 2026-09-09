import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'

import App from './App.vue'
import router from './router'
import { bootstrapSession } from './store/auth'
import './styles/index.css'

const app = createApp(App)

for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(createPinia())
app.use(router)
app.use(ElementPlus, { locale: zhCn })

// 访问令牌只在内存，刷新页面后必须先静默续期再挂载：
// 路由守卫依赖 getToken()，若先挂载会被判为未登录而跳到 /login，
// 续期成功后又跳回业务页 —— 那才是真正的闪屏。await 后再 mount
// 只产生一次导航；等待期间由 index.html 的静态 loading 占位兜底，不会白屏。
bootstrapSession().finally(() => {
  app.mount('#app')
})
