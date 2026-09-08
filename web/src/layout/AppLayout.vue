<template>
  <el-container class="layout">
    <el-aside :width="collapsed ? '64px' : '210px'" class="aside">
      <div class="logo">
        <el-icon :size="22" color="#6aa1ff"><FirstAidKit /></el-icon>
        <span v-show="!collapsed">AutoMedic</span>
      </div>
      <el-menu
        :default-active="activePath"
        :collapse="collapsed"
        background-color="transparent"
        text-color="#a7b0c2"
        active-text-color="#6aa1ff"
        @select="onMenuSelect"
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
          <el-tag size="small" type="info" effect="plain" class="dsh-tag">dsh --profile headless</el-tag>
          <el-dropdown trigger="click" @command="onCommand">
            <span class="user-chip">
              <el-icon :size="16"><UserFilled /></el-icon>
              <span class="user-name">{{ displayName }}</span>
              <span v-if="isSuper" class="am-pill am-pill-current">超管</span>
              <el-icon :size="12"><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <div class="user-meta">
                  <div class="user-meta-name">{{ displayName }}</div>
                  <div class="user-meta-sub">
                    {{ roleNames || '未分配角色' }} · 租户 #{{ state.user?.tenant_id || '-' }}
                  </div>
                </div>
                <el-dropdown-item command="password">修改密码</el-dropdown-item>
                <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>

  <el-dialog v-model="pwdDialog" title="修改密码" width="420" class="am-dialog">
    <p class="am-dialog-desc">修改后当前会话仍然有效，下次登录请使用新密码。</p>
    <el-form :model="pwd" label-position="top" class="am-form">
      <el-form-item label="原密码">
        <el-input v-model="pwd.old_password" type="password" show-password />
      </el-form-item>
      <el-form-item label="新密码">
        <el-input v-model="pwd.new_password" type="password" show-password placeholder="至少 6 位" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button class="am-btn-soft" @click="pwdDialog = false">取消</el-button>
      <el-button type="primary" class="am-btn-primary" :loading="pwdLoading" @click="submitPassword">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowDown, UserFilled, FirstAidKit } from '@element-plus/icons-vue'
import { visibleMenus } from '@/router'
import { changePassword, logout } from '@/api'
import { useAuth } from '@/store/auth'
import { ElMessage } from 'element-plus'

const route = useRoute()
const router = useRouter()
const { state, isSuper, reset } = useAuth()

const collapsed = ref(false)
const pwdDialog = ref(false)
const pwdLoading = ref(false)
const pwd = ref({ old_password: '', new_password: '' })

const menus = computed(() => visibleMenus())
const displayName = computed(() => state.user?.display_name || state.user?.username || '未登录')
const roleNames = computed(() => (state.user?.roles || []).join('、'))

const activePath = computed(() => {
  const p = route.path
  if (p.startsWith('/projects') && p !== '/projects') return '/projects'
  if (p.startsWith('/tasks') && p !== '/tasks') return '/tasks'
  return p
})

function onMenuSelect(index) {
  if (route.path === index) return
  router.push(index)
}

async function onCommand(cmd) {
  if (cmd === 'password') {
    pwd.value = { old_password: '', new_password: '' }
    pwdDialog.value = true
  } else if (cmd === 'logout') {
    try { await logout() } catch { /* 忽略：本地会话同样清理 */ }
    reset()
    ElMessage.success('已退出登录')
    router.replace('/login')
  }
}

async function submitPassword() {
  if (pwd.value.new_password.length < 6) {
    ElMessage.error('新密码至少 6 位')
    return
  }
  pwdLoading.value = true
  try {
    await changePassword(pwd.value)
    ElMessage.success('密码已更新')
    pwdDialog.value = false
  } finally {
    pwdLoading.value = false
  }
}
</script>

<style scoped>
.layout { height: 100vh; }
.aside { background: #12141a; border-right: 1px solid var(--am-border); transition: width .2s; overflow: hidden; }
.logo { height: 56px; display: flex; align-items: center; gap: 8px; padding: 0 18px; color: var(--am-text); font-weight: 600; font-size: 15px; }
.header { display: flex; align-items: center; justify-content: space-between; background: var(--am-bg-elevated); border-bottom: 1px solid var(--am-border); }
.header-left { display: flex; align-items: center; gap: 12px; }
.header-right { display: flex; align-items: center; gap: 12px; }
.main { background: var(--am-bg); padding: 0; overflow-y: auto; }
.dsh-tag { border-radius: 999px; }

.user-chip {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 5px 10px; border-radius: 999px;
  background: var(--am-bg-inset); border: 1px solid var(--am-border);
  cursor: pointer; font-size: 13px; color: var(--am-text);
  transition: border-color .18s ease;
}
.user-chip:hover { border-color: var(--am-border-strong); }
.user-name { max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.user-meta { padding: 8px 14px 10px; border-bottom: 1px solid var(--am-border); margin-bottom: 4px; }
.user-meta-name { font-size: 13.5px; font-weight: 600; }
.user-meta-sub { margin-top: 2px; font-size: 12px; color: var(--am-text-dim); }
:deep(.el-menu) { border-right: none; }
</style>
