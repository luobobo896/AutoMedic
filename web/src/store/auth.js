import { computed, reactive } from 'vue'
import { cachedUser, clearSession, getToken, setSession } from '@/api'

const state = reactive({
  user: cachedUser() || null,
  // 超管可切换查看的租户（0 表示全部/本租户）
  activeTenant: 0
})

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
