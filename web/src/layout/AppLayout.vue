<template>
  <el-container class="layout" :class="{ 'is-compact': compact, 'is-nav-open': compact && !collapsed }">
    <div v-if="compact && !collapsed" class="nav-scrim" @click="collapsed = true" />
    <el-aside v-show="!(compact && collapsed)" :width="asideWidth" class="aside">
      <div class="logo">
        <span class="logo-icon"><el-icon :size="16"><FirstAidKit /></el-icon></span>
        <span v-show="!collapsed || compact" class="logo-text">AutoMedic<em>自愈平台</em></span>
      </div>
      <el-menu
        :default-active="activePath"
        :collapse="collapsed && !compact"
        class="nav"
        @select="onMenuSelect"
      >
        <el-menu-item-group v-for="g in menuGroups" :key="g.name" :title="g.name">
          <el-menu-item v-for="m in g.items" :key="m.path" :index="m.path">
            <el-icon><component :is="m.meta.icon" /></el-icon>
            <template #title>{{ m.meta.title }}</template>
          </el-menu-item>
        </el-menu-item-group>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="header">
        <div class="header-left">
          <el-button text :icon="collapsed ? 'Expand' : 'Fold'" :aria-label="collapsed ? '展开侧栏' : '收起侧栏'" @click="collapsed = !collapsed" />
        </div>
        <div class="header-right">
          <el-tag size="small" type="info" effect="plain" class="dsh-tag" title="当前 dsh 运行档：headless">dsh · headless</el-tag>
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

      <el-main class="main" :class="{ 'main-fill': fillMain }">
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
      <el-button @click="pwdDialog = false">取消</el-button>
      <el-button type="primary" :loading="pwdLoading" @click="submitPassword">保存</el-button>
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
import { useBreakpoint } from '@/composables/useBreakpoint'
import { MQ } from '@/constants/breakpoints'
import { ElMessage } from 'element-plus'

const route = useRoute()
const router = useRouter()
const { state, isSuper, reset } = useAuth()
const { isMobile, isTablet } = useBreakpoint()

// <768px 抽屉侧栏；768–1023px 收起为图标列；≥1024px 展开。
// 断点与 CSS 同源（constants/breakpoints.js）；跨越 1024px 时自动收起/展开，
// 同带宽内的手动折叠不被覆盖。
const compact = isMobile
const collapsed = ref(window.matchMedia(MQ.ltDesktop).matches)
watch(
  () => isMobile.value || isTablet.value,
  (narrow) => { collapsed.value = narrow }
)
const asideWidth = computed(() => {
  if (compact.value) return '210px'
  return collapsed.value ? '64px' : '210px'
})
const pwdDialog = ref(false)
const pwdLoading = ref(false)
const pwd = ref({ old_password: '', new_password: '' })

const menus = computed(() => visibleMenus())

// 业务分组：把 16 个平铺菜单按「看盘 → 接入 → 配置 → 管理」收敛，
// 新用户一眼看到主链路，管理配置类不再和业务页抢视线。
const MENU_GROUPS = [
  { name: '运营', paths: ['/', '/events', '/tasks', '/statistics'] },
  { name: '接入', paths: ['/projects', '/repos', '/rules', '/tokens', '/credentials'] },
  { name: '配置', paths: ['/models', '/settings', '/dicts'] },
  { name: '系统管理', paths: ['/users', '/roles', '/tenants'] }
]
const menuGroups = computed(() => {
  const rest = [...menus.value]
  const groups = MENU_GROUPS.map((g) => {
    const items = g.paths
      .map((p) => rest.find((m) => m.path === p))
      .filter(Boolean)
    items.forEach((m) => rest.splice(rest.indexOf(m), 1))
    return { name: g.name, items }
  }).filter((g) => g.items.length)
  if (rest.length) groups.push({ name: '其他', items: rest })
  return groups
})
const displayName = computed(() => state.user?.display_name || state.user?.username || '未登录')
const roleNames = computed(() => (state.user?.roles || []).join('、'))
const fillMain = computed(() => route.path.startsWith('/tasks/') && route.path !== '/tasks')

const activePath = computed(() => {
  const p = route.path
  if (p.startsWith('/projects') && p !== '/projects') return '/projects'
  if (p.startsWith('/tasks') && p !== '/tasks') return '/tasks'
  return p
})

function onMenuSelect(index) {
  if (compact.value) collapsed.value = true
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
.layout { height: 100vh; height: 100dvh; overflow: hidden; position: relative; }
.layout > .el-container {
  flex: 1 1 0;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.aside { background: var(--am-nav); border-right: none; transition: width var(--am-duration); overflow: hidden; }
.nav-scrim {
  position: absolute;
  inset: 0;
  z-index: var(--am-z-nav);
  background: var(--am-scrim);
}
.layout.is-compact .aside {
  position: absolute;
  left: 0;
  top: 0;
  z-index: var(--am-z-overlay);
  height: 100%;
  box-shadow: var(--am-shadow);
}
.logo {
  height: 56px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 var(--am-space-4);
  color: var(--am-nav-text-strong);
  font-weight: 600;
  font-size: var(--am-font-lg);
  border-bottom: 1px solid var(--am-nav-border);
  white-space: nowrap;
}
.logo-icon {
  flex: none;
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--am-radius-sm);
  background: var(--am-primary);
  color: var(--am-on-accent);
}
.logo-text { display: flex; align-items: baseline; gap: 6px; }
.logo-text em {
  font-style: normal;
  font-size: var(--am-font-xs);
  font-weight: 400;
  color: var(--am-nav-group);
}
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex: none;
  height: 56px;
  background: var(--am-bg-elevated);
  border-bottom: 1px solid var(--am-border);
}
.header-left { display: flex; align-items: center; gap: 12px; }
.header-left :deep(.el-button) { min-width: 44px; min-height: 44px; }
.header-right { display: flex; align-items: center; gap: 12px; }
.main {
  background: var(--am-bg);
  padding: 0;
  padding-bottom: env(safe-area-inset-bottom, 0px);
  --el-main-padding: 0;
  overflow-x: hidden;
  overflow-y: auto;
}
.main.main-fill {
  flex: 1 1 0;
  min-height: 0;
  height: calc(100vh - 56px);
  height: calc(100dvh - 56px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.main.main-fill :deep(.am-task-detail) {
  flex: 1 1 0;
  min-height: 0;
  height: 100%;
}
.dsh-tag { border-radius: 999px; }
@media (max-width: 767.98px) {
  .dsh-tag { display: none; }
  .user-name { max-width: 72px; }
}

.user-chip {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 4px 10px 4px 4px; border-radius: 999px;
  background: var(--am-bg-elevated); border: 1px solid var(--am-border);
  cursor: pointer; font-size: var(--am-font-sm); color: var(--am-text);
  transition: border-color .18s ease;
}
.user-chip:hover { border-color: var(--am-border-strong); }
.user-chip :deep(.el-icon:first-child) {
  width: 26px; height: 26px; border-radius: 50%;
  background: var(--am-primary-soft); color: var(--am-primary);
  display: flex; align-items: center; justify-content: center;
}
.user-name { max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.user-meta { padding: 8px 14px 10px; border-bottom: 1px solid var(--am-border); margin-bottom: 4px; }
.user-meta-name { font-size: var(--am-font-md); font-weight: 600; }
.user-meta-sub { margin-top: 2px; font-size: var(--am-font-xs); color: var(--am-text-dim); }
.nav { border-right: none; background: transparent; padding-bottom: var(--am-space-3); }
.nav :deep(.el-menu-item-group__title) {
  padding: var(--am-space-4) var(--am-space-4) 6px;
  font-size: var(--am-font-xs);
  line-height: 1.4;
  color: var(--am-nav-group);
  letter-spacing: .04em;
}
.nav :deep(.el-menu-item) {
  height: 44px;
  line-height: 44px;
  margin: 2px var(--am-space-2);
  border-radius: var(--am-radius-sm);
  color: var(--am-nav-text);
  font-size: var(--am-font-md);
  transition: background-color var(--am-duration) var(--am-easing), color var(--am-duration) var(--am-easing);
}
.nav :deep(.el-menu-item .el-icon) { width: 18px; font-size: var(--am-font-lg); }
.nav :deep(.el-menu-item:hover),
.nav :deep(.el-menu-item:focus) { background: var(--am-nav-hover); color: var(--am-nav-text-strong); }
.nav :deep(.el-menu-item.is-active) { background: var(--am-primary); color: var(--am-nav-text-strong); font-weight: 600; }
.nav :deep(.el-menu--collapse .el-menu-item) { margin: 2px var(--am-space-1); }
.nav :deep(.el-menu-item:focus-visible) { outline: 2px solid var(--am-nav-text-strong); outline-offset: -2px; }
</style>
