import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

const REFRESH_KEY = 'automedic_refresh_token'
const USER_KEY = 'automedic_user'

// 访问令牌只保存在内存，避免 XSS 直接从 localStorage 读走；
// 页面刷新后由 restoreSession() 用 refresh_token 静默续期重建。
let accessToken = ''

export function getToken() {
  return accessToken
}
export function getRefreshToken() {
  return localStorage.getItem(REFRESH_KEY) || ''
}
export function setSession(token, refreshToken, user) {
  if (token) accessToken = token
  if (refreshToken) localStorage.setItem(REFRESH_KEY, refreshToken)
  if (user) localStorage.setItem(USER_KEY, JSON.stringify(user))
}
export function clearSession() {
  accessToken = ''
  localStorage.removeItem(REFRESH_KEY)
  localStorage.removeItem(USER_KEY)
}
export function cachedUser() {
  try {
    return JSON.parse(localStorage.getItem(USER_KEY) || 'null')
  } catch {
    return null
  }
}

const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || '/api',
  timeout: 60000
})

http.interceptors.request.use((cfg) => {
  const t = getToken()
  if (t) cfg.headers.Authorization = 'Bearer ' + t
  return cfg
})

// 401 自动续期一次，失败则清理会话并跳登录
let refreshing = null
// 并发 401 只清一次会话、只跳一次登录页
let sessionExpiredHandled = false

function handleSessionExpired(notify) {
  if (sessionExpiredHandled) return
  sessionExpiredHandled = true
  clearSession()
  if (notify) ElMessage.error('登录已失效，请重新登录')
  if (router.currentRoute.value.path !== '/login') {
    router.replace('/login').catch(() => { /* ignore */ })
  }
  // 同一轮事件循环内的并发 401 合并处理，跳转后重置标志
  setTimeout(() => { sessionExpiredHandled = false }, 0)
}

http.interceptors.response.use(
  (res) => {
    const data = res.data
    if (data && typeof data === 'object' && 'code' in data) {
      if (data.code === 0) return data
      ElMessage.error(data.message || '请求失败')
      return Promise.reject(new Error(data.message || '请求失败'))
    }
    return data
  },
  async (err) => {
    if (err?.silent) return Promise.reject(err)
    const cfg = err.config || {}
    const status = err?.response?.status
    const msg = err?.response?.data?.message || err.message || '网络错误'
    // 续期/登录接口自身 401 绝不再触发续期，否则会递归打服务端
    const url = cfg.url || ''
    const isAuthEndpoint = url.includes('/auth/refresh') || url.includes('/auth/login')

    if (status === 401 && !cfg._retry && !isAuthEndpoint && getRefreshToken()) {
      cfg._retry = true
      try {
        if (!refreshing) {
          refreshing = refreshToken()
            .then((r) => {
              setSession(r.data.token, r.data.refresh_token, r.data.user)
              return r.data.token
            })
            .finally(() => { refreshing = null })
        }
        const token = await refreshing
        cfg.headers = cfg.headers || {}
        cfg.headers.Authorization = 'Bearer ' + token
        return http.request(cfg)
      } catch {
        if (!cfg._silent) handleSessionExpired(true)
        return Promise.reject(err)
      }
    }

    // 静默请求（引导期续期）不弹提示、不跳转，由调用方自行处理
    if (cfg._silent) return Promise.reject(err)

    if (status === 401) {
      handleSessionExpired(true)
    } else if (status === 403) {
      ElMessage.error(msg || '无操作权限')
    } else {
      ElMessage.error(msg)
    }
    return Promise.reject(err)
  }
)

// ---------- 认证 ----------
export const login = (data) => http.post('/v1/auth/login', data)
// _retry: true 是硬保险 —— 刷新请求本身永远不会再进入续期分支
export const refreshToken = (opts = {}) =>
  http.post('/v1/auth/refresh', { refresh_token: getRefreshToken() }, { _retry: true, ...opts })
export const logout = () => http.post('/v1/auth/logout', { refresh_token: getRefreshToken() })

// 应用启动时的静默续期：内存无访问令牌但 localStorage 有 refresh_token 时重建会话
// 返回最新的 user（供 store 同步权限），失败返回 null
export async function restoreSession() {
  if (accessToken) return cachedUser()
  if (!getRefreshToken()) return null
  try {
    const r = await refreshToken({ _silent: true })
    const user = r.data.user || cachedUser()
    setSession(r.data.token, r.data.refresh_token, user)
    return user || null
  } catch {
    clearSession()
    return null
  }
}
export const profile = () => http.get('/v1/auth/profile')
export const changePassword = (data) => http.put('/v1/auth/password', data)

// ---------- 用户 / 角色 / 租户 ----------
export const listUsers = (params) => http.get('/v1/users', { params })
export const createUser = (data) => http.post('/v1/users', data)
export const updateUser = (id, data) => http.put(`/v1/users/${id}`, data)
export const deleteUser = (id) => http.delete(`/v1/users/${id}`)

export const listRoles = () => http.get('/v1/roles')
export const createRole = (data) => http.post('/v1/roles', data)
export const updateRole = (id, data) => http.put(`/v1/roles/${id}`, data)
export const deleteRole = (id) => http.delete(`/v1/roles/${id}`)
export const listPermissions = () => http.get('/v1/permissions')

export const listTenants = () => http.get('/v1/tenants')
export const createTenant = (data) => http.post('/v1/tenants', data)
export const updateTenant = (id, data) => http.put(`/v1/tenants/${id}`, data)
export const deleteTenant = (id) => http.delete(`/v1/tenants/${id}`)

// ---------- 概览 ----------
export const overview = () => http.get('/v1/overview')

// ---------- 项目 ----------
export const listProjects = (params) => http.get('/v1/projects', { params })
export const createProject = (data) => http.post('/v1/projects', data)
export const getProject = (id) => http.get(`/v1/projects/${id}`)
export const updateProject = (id, data) => http.put(`/v1/projects/${id}`, data)
export const deleteProject = (id) => http.delete(`/v1/projects/${id}`)

// ---------- 仓库 ----------
export const listRepos = (params) => http.get('/v1/repos', { params })
export const createRepo = (data) => http.post('/v1/repos', data)
export const updateRepo = (id, data) => http.put(`/v1/repos/${id}`, data)
export const deleteRepo = (id) => http.delete(`/v1/repos/${id}`)
export const testRepo = (id) => http.post(`/v1/repos/${id}/test`)
export const repoTree = (id) => http.get(`/v1/repos/${id}/tree`)
export const repoFile = (id, path) => http.get(`/v1/repos/${id}/file`, { params: { path } })
export const startRepoReview = (id, data) => http.post(`/v1/repos/${id}/review`, data, { timeout: 120000 })
export const listRepoReviews = (id) => http.get(`/v1/repos/${id}/reviews`)
export const getReviewJob = (id) => http.get(`/v1/reviews/${id}`, { timeout: 30000 })
export const fixReviewJob = (id, data) => http.post(`/v1/reviews/${id}/fix`, data, { timeout: 120000 })

// ---------- 凭证 ----------
export const listCredentials = () => http.get('/v1/credentials')
export const createCredential = (data) => http.post('/v1/credentials', data)
export const updateCredential = (id, data) => http.put(`/v1/credentials/${id}`, data)
export const deleteCredential = (id) => http.delete(`/v1/credentials/${id}`)
export const listCredentialUsages = (params) => http.get('/v1/credential-usages', { params })

// ---------- 大模型 ----------
export const listLLMConfig = () => http.get('/v1/providers')
export const createProvider = (data) => http.post('/v1/providers', data)
export const updateProvider = (id, data) => http.put(`/v1/providers/${id}`, data)
export const deleteProvider = (id) => http.delete(`/v1/providers/${id}`)
export const listModels = (params) => http.get('/v1/models', { params })
export const createModel = (data) => http.post('/v1/models', data)
export const updateModel = (id, data) => http.put(`/v1/models/${id}`, data)
export const deleteModel = (id) => http.delete(`/v1/models/${id}`)

// ---------- 令牌 ----------
export const listTokens = (params) => http.get('/v1/tokens', { params })
export const createToken = (data) => http.post('/v1/tokens', data)
export const updateToken = (id, data) => http.put(`/v1/tokens/${id}`, data)
export const deleteToken = (id) => http.delete(`/v1/tokens/${id}`)

// ---------- 规则 ----------
export const listRules = (params) => http.get('/v1/rules', { params })
export const createRule = (data) => http.post('/v1/rules', data)
export const updateRule = (id, data) => http.put(`/v1/rules/${id}`, data)
export const deleteRule = (id) => http.delete(`/v1/rules/${id}`)

// ---------- 事件 ----------
export const listEvents = (params) => http.get('/v1/events', { params })
export const getEvent = (id) => http.get(`/v1/events/${id}`)
export const replayEvent = (id) => http.post(`/v1/events/${id}/replay`)

function intID(id) {
  const n = Number(id)
  return Number.isInteger(n) && n > 0 ? n : 0
}
function withTaskID(id, fn) {
  const n = intID(id)
  if (!n) {
    const err = new Error('skip')
    err.silent = true
    return Promise.reject(err)
  }
  return fn(n)
}

// ---------- 任务 ----------
export const listTasks = (params) => http.get('/v1/tasks', { params })
export const getTask = (id) => withTaskID(id, n => http.get(`/v1/tasks/${n}`))
export const retryTask = (id) => withTaskID(id, n => http.post(`/v1/tasks/${n}/retry`))
export const cancelTask = (id) => withTaskID(id, n => http.post(`/v1/tasks/${n}/cancel`))
export const ignoreTask = (id) => withTaskID(id, n => http.post(`/v1/tasks/${n}/ignore`))
export const confirmTask = (id, data) => withTaskID(id, n => http.post(`/v1/tasks/${n}/confirm`, data))
export const rejectTask = (id, data) => withTaskID(id, n => http.post(`/v1/tasks/${n}/reject`, data))
export const taskLogs = (id, params) => withTaskID(id, n => http.get(`/v1/tasks/${n}/logs`, { params }))
export const taskPatch = (id) => withTaskID(id, n => http.get(`/v1/tasks/${n}/patch`))

// ---------- 统计 ----------
export const statsOverview = (params) => http.get('/v1/stats/overview', { params })
export const statsTrend = (params) => http.get('/v1/stats/trend', { params })
export const statsGroup = (params) => http.get('/v1/stats/group', { params })

// ---------- 设置 ----------
export const getSettings = () => http.get('/v1/settings')
export const updateSettings = (data) => http.put('/v1/settings', data)
export const listDicts = (params) => http.get('/v1/dicts', { params })
export const createDict = (data) => http.post('/v1/dicts', data)
export const updateDict = (id, data) => http.put(`/v1/dicts/${id}`, data)
export const deleteDict = (id) => http.delete(`/v1/dicts/${id}`)

export function taskWSURL(taskId) {
  const n = intID(taskId)
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const root = String(import.meta.env.BASE_URL || '/').replace(/\/?$/, '/')
  return `${proto}//${location.host}${root}ws/tasks/${n}`
}

// 令牌通过 Sec-WebSocket-Protocol 子协议传递，不再出现在 URL 里
// （URL 会被代理/网关日志与浏览器历史记录留存）。
// 必须同时提供裸 'automedic'：服务端回选的是 'automedic'，而浏览器会校验
// 服务端选中的子协议必须在客户端提供的列表内，否则握手直接失败。
export function createTaskWS(taskId) {
  const n = intID(taskId)
  const token = getToken()
  if (!n || !token) return null
  return new WebSocket(taskWSURL(n), ['automedic.' + token, 'automedic'])
}

export default http
