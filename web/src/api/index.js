import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

const TOKEN_KEY = 'automedic_token'
const REFRESH_KEY = 'automedic_refresh_token'
const USER_KEY = 'automedic_user'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || ''
}
export function getRefreshToken() {
  return localStorage.getItem(REFRESH_KEY) || ''
}
export function setSession(token, refreshToken, user) {
  if (token) localStorage.setItem(TOKEN_KEY, token)
  if (refreshToken) localStorage.setItem(REFRESH_KEY, refreshToken)
  if (user) localStorage.setItem(USER_KEY, JSON.stringify(user))
}
export function clearSession() {
  localStorage.removeItem(TOKEN_KEY)
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
    const status = err?.response?.status
    const msg = err?.response?.data?.message || err.message || '网络错误'
    if (status === 401 && !err.config?._retry && getRefreshToken()) {
      err.config._retry = true
      try {
        refreshing = refreshing || refreshToken()
        const r = await refreshing
        setSession(r.data.token, r.data.refresh_token, r.data.user)
        refreshing = null
        err.config.headers.Authorization = 'Bearer ' + r.data.token
        return http.request(err.config)
      } catch (e) {
        refreshing = null
        clearSession()
        if (router.currentRoute.value.path !== '/login') router.replace('/login')
      }
    }
    if (status === 401) {
      clearSession()
      if (router.currentRoute.value.path !== '/login') router.replace('/login')
      ElMessage.error('登录已失效，请重新登录')
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
export const refreshToken = () => http.post('/v1/auth/refresh', { refresh_token: getRefreshToken() })
export const logout = () => http.post('/v1/auth/logout', { refresh_token: getRefreshToken() })
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
  return `${proto}//${location.host}${root}ws/tasks/${n}?token=${encodeURIComponent(getToken())}`
}

export default http
