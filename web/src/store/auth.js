import { computed, reactive } from 'vue'
import { cachedUser, clearSession, getToken, restoreSession, setSession } from '@/api'

const state = reactive({
  user: cachedUser() || null,
  // 超管可切换查看的租户（0 表示全部/本租户）
  activeTenant: 0
})

// 应用挂载前调用：访问令牌只在内存，页面刷新后必须用 refresh_token 续期，
// 同时用服务端返回的 user 覆盖 localStorage 里可能已过期的权限快照。
export async function bootstrapSession() {
  const user = await restoreSession()
  state.user = user || null
  return !!user
}

export function useAuth() {
  const logged = computed(() => !!getToken() && !!state.user)
  const permissions = computed(() => state.user?.permissions || [])
  const isSuper = computed(() => !!state.user?.is_super)
  const roles = computed(() => state.user?.roles || [])

  function can(code) {
    if (isSuper.value) return true
    return permissions.value.includes(code)
  }

  function applySession(token, refreshToken, user) {
    setSession(token, refreshToken, user)
    state.user = user
  }

  function reset() {
    clearSession()
    state.user = null
  }

  return { state, logged, permissions, roles, isSuper, can, applySession, reset }
}
