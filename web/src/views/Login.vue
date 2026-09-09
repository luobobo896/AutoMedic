<template>
  <div class="login-page">
    <div class="login-card">
      <div class="brand">
        <el-icon :size="26" class="brand-icon"><FirstAidKit /></el-icon>
        <div>
          <div class="brand-name">AutoMedic</div>
          <div class="brand-sub">事件驱动的自动修复平台</div>
        </div>
      </div>

      <h1 class="login-title">账号登录</h1>
      <p class="login-desc">使用账号密码登录，权限由所属租户与角色决定。</p>

      <el-form :model="form" @keyup.enter="submit" class="login-form">
        <el-form-item label="账号">
          <el-input v-model="form.username" size="large" placeholder="请输入账号" :prefix-icon="'User'" autocomplete="username" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="form.password" size="large" type="password" show-password placeholder="请输入密码"
            :prefix-icon="'Lock'" autocomplete="current-password" />
        </el-form-item>
        <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon class="login-alert" />
        <el-button type="primary" size="large" class="login-btn" :loading="loading" @click="submit">
          {{ loading ? '登录中…' : '登录' }}
        </el-button>
      </el-form>

      <div v-if="showDefaultHint" class="login-foot">默认账号 admin / admin123（仅开发环境）</div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { FirstAidKit } from '@element-plus/icons-vue'
import { login, profile } from '@/api'
import { useAuth } from '@/store/auth'

const router = useRouter()
const route = useRoute()
const { applySession } = useAuth()

// 生产构建不暴露默认口令
const showDefaultHint = import.meta.env.DEV

const form = ref({ username: '', password: '' })
const loading = ref(false)
const error = ref('')

async function submit() {
  if (!form.value.username || !form.value.password) {
    error.value = '请输入账号与密码'
    return
  }
  loading.value = true
  error.value = ''
  try {
    const r = await login(form.value)
    let user = r.data?.user
    if (!user) {
      const p = await profile()
      user = p.data
    }
    applySession(r.data.token, r.data.refresh_token, user)
    const redirect = route.query.redirect && route.query.redirect !== '/login' ? route.query.redirect : '/'
    router.replace(redirect)
  } catch (e) {
    error.value = e?.response?.data?.message || e?.message || '登录失败'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  /* dvh：iOS Safari 地址栏收起时不再遮挡表单 */
  min-height: 100dvh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--am-bg);
  padding: var(--am-space-5);
}
.login-card {
  width: 100%;
  max-width: 420px;
  background: var(--am-bg-elevated);
  border: 1px solid var(--am-border);
  border-radius: var(--am-radius-lg);
  padding: 32px 32px 24px;
  box-shadow: var(--am-shadow);
}
.brand { display: flex; align-items: center; gap: var(--am-space-3); margin-bottom: var(--am-space-5); }
.brand-icon { color: var(--am-primary); }
.brand-name { font-size: var(--am-font-lg); font-weight: 700; letter-spacing: .2px; }
.brand-sub { font-size: var(--am-font-xs); color: var(--am-text-dim); margin-top: 2px; }
.login-title { margin: 0; font-size: var(--am-font-2xl); font-weight: 700; }
.login-desc { margin: 6px 0 20px; font-size: var(--am-font-sm); color: var(--am-text-dim); line-height: var(--am-leading-relaxed); }
.login-form :deep(.el-form-item__label) { font-weight: 500; padding-bottom: 6px; }
.login-btn { width: 100%; margin-top: 4px; border-radius: var(--am-radius-md); font-weight: 600; }
.login-alert { margin-bottom: var(--am-space-3); border-radius: var(--am-radius-md); }
.login-foot { margin-top: 18px; text-align: center; font-size: var(--am-font-xs); color: var(--am-text-dim); }
</style>
