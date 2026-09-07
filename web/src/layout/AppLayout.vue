<template>
  <el-container class="layout">
    <el-aside :width="collapsed ? '64px' : '210px'" class="aside">
      <div class="logo">
        <el-icon :size="22" color="#4f8cff"><FirstAidKit /></el-icon>
        <span v-show="!collapsed">AutoMedic</span>
      </div>
      <el-menu
        :default-active="activePath"
        :collapse="collapsed"
        background-color="#121826"
        text-color="#a7b0c2"
        active-text-color="#4f8cff"
        router
      >
        <el-menu-item v-for="m in menus" :key="m.path" :index="m.path">
          <el-icon><component :is="m.meta.icon" /></el-icon>
          <template #title>{{ m.meta.title }}</template>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="header">
        <div class="header-left">
          <el-button text :icon="collapsed ? 'Expand' : 'Fold'" @click="collapsed = !collapsed" />
          <el-breadcrumb separator="/">
            <el-breadcrumb-item>AutoMedic</el-breadcrumb-item>
            <el-breadcrumb-item>{{ route.meta.title || '' }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
          <el-tag size="small" type="info" effect="dark">dsh --profile headless</el-tag>
          <el-popover placement="bottom-end" :width="340" trigger="click">
            <template #reference>
              <el-button text :icon="'Setting'" />
            </template>
            <div>
              <div class="setting-row">
                <span>管理令牌（X-Admin-Token）</span>
                <el-input v-model="token" size="small" type="password" show-password placeholder="change-me" />
              </div>
              <div class="setting-row">
                <el-button size="small" type="primary" @click="saveToken">保存</el-button>
                <el-button size="small" @click="loadHealth">测试连接</el-button>
              </div>
              <div class="setting-row am-text-dim" style="font-size:12px">
                {{ healthText }}
              </div>
            </div>
          </el-popover>
        </div>
      </el-header>

      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { menus } from '@/router'
import { getToken, setToken } from '@/api'
import axios from 'axios'

const route = useRoute()
const collapsed = ref(false)
const token = ref(getToken())
const healthText = ref('')

const activePath = computed(() => {
  const p = route.path
  if (p.startsWith('/projects') && p !== '/projects') return '/projects'
  if (p.startsWith('/tasks') && p !== '/tasks') return '/tasks'
  return p
})

function saveToken() {
  setToken(token.value)
  healthText.value = '已保存到本地'
}

async function loadHealth() {
  try {
    const base = import.meta.env.VITE_API_BASE || '/api'
    const r = await axios.get(base.replace('/api', '') + '/healthz', { timeout: 5000 })
    healthText.value = '服务状态：' + JSON.stringify(r.data)
  } catch (e) {
    healthText.value = '连接失败：' + e.message
  }
}

onMounted(loadHealth)
</script>

<style scoped>
.layout { height: 100vh; }
.aside { background: #121826; border-right: 1px solid var(--am-border); transition: width .2s; overflow: hidden; }
.logo { height: 56px; display: flex; align-items: center; gap: 8px; padding: 0 18px; color: var(--am-text); font-weight: 600; font-size: 15px; }
.header { display: flex; align-items: center; justify-content: space-between; background: var(--am-bg-elevated); border-bottom: 1px solid var(--am-border); }
.header-left { display: flex; align-items: center; gap: 12px; }
.header-right { display: flex; align-items: center; gap: 10px; }
.main { background: var(--am-bg); padding: 0; overflow-y: auto; }
.setting-row { margin-bottom: 10px; }
:deep(.el-menu) { border-right: none; }
</style>
