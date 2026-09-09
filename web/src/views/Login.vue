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

      <div class="login-foot">默认账号 admin / admin123，首次登录后请尽快修改密码</div>
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
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--am-bg);
  padding: 24px;
}
.login-card {
  width: 100%;
  max-width: 420px;
  background: var(--am-bg-elevated);
  border: 1px solid var(--am-border);
  border-radius: 18px;
  padding: 32px 32px 24px;
  box-shadow: 0 24px 60px rgba(0, 0, 0, .35);
}
.brand { display: flex; align-items: center; gap: 12px; margin-bottom: 24px; }
.brand-icon { color: var(--am-primary); }
.brand-name { font-size: 17px; font-weight: 700; letter-spacing: .2px; }
.brand-sub { font-size: 12px; color: var(--am-text-dim); margin-top: 2px; }
.login-title { margin: 0; font-size: 22px; font-weight: 700; }
.login-desc { margin: 6px 0 20px; font-size: 13px; color: var(--am-text-dim); line-height: 1.6; }
.login-form :deep(.el-form-item__label) { font-weight: 500; padding-bottom: 6px; }
.login-btn { width: 100%; margin-top: 4px; border-radius: 12px; font-weight: 600; }
.login-alert { margin-bottom: 12px; border-radius: 10px; }
.login-foot { margin-top: 18px; text-align: center; font-size: 12px; color: var(--am-text-dim); }
</style>
