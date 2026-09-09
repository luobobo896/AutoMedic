<template>
  <div class="am-page forbidden-page">
    <div class="am-card">
      <el-empty description="当前账号没有任何可访问的功能页">
        <template #image>
          <el-icon :size="64" color="var(--am-danger)"><Lock /></el-icon>
        </template>
        <div class="forbidden-tip">
          请联系管理员为你所属角色分配菜单权限（如 overview:read、task:read）。
        </div>
        <div class="forbidden-actions">
          <el-button type="primary" @click="backToLogin">返回登录</el-button>
          <el-button @click="doLogout">退出登录</el-button>
        </div>
      </el-empty>
    </div>
  </div>
</template>

<script setup>
import { useRouter } from 'vue-router'
import { Lock } from '@element-plus/icons-vue'
import { logout } from '@/api'
import { useAuth } from '@/store/auth'

const router = useRouter()
const { reset } = useAuth()

function backToLogin() {
  router.replace('/login')
}

async function doLogout() {
  try { await logout() } catch { /* 忽略：本地会话同样清理 */ }
  reset()
  router.replace('/login')
}
</script>

<style scoped>
.forbidden-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 60vh;
}
.forbidden-page .am-card {
  width: 100%;
  max-width: 520px;
}
.forbidden-tip {
  font-size: var(--am-font-sm);
  color: var(--am-text-dim);
  margin-bottom: 16px;
  line-height: 1.7;
}
.forbidden-actions {
  display: flex;
  gap: 12px;
  justify-content: center;
}
</style>
