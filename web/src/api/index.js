import axios from 'axios'
import { ElMessage } from 'element-plus'

const TOKEN_KEY = 'automedic_admin_token'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || import.meta.env.VITE_ADMIN_TOKEN || ''
}

export function setToken(t) {
  if (t) localStorage.setItem(TOKEN_KEY, t)
  else localStorage.removeItem(TOKEN_KEY)
}

const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || '/api',
  timeout: 60000
})

http.interceptors.request.use((cfg) => {
  cfg.headers['X-Admin-Token'] = getToken()
  return cfg
})

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
  (err) => {
    const msg = err?.response?.data?.message || err.message || '网络错误'
    if (err?.response?.status === 401) ElMessage.error('管理令牌无效，请在右上角设置中更新')
    else ElMessage.error(msg)
    return Promise.reject(err)
  }
)

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

// ---------- 任务 ----------
export const listTasks = (params) => http.get('/v1/tasks', { params })
export const getTask = (id) => http.get(`/v1/tasks/${id}`)
export const retryTask = (id) => http.post(`/v1/tasks/${id}/retry`)
export const cancelTask = (id) => http.post(`/v1/tasks/${id}/cancel`)
export const ignoreTask = (id) => http.post(`/v1/tasks/${id}/ignore`)
export const confirmTask = (id, data) => http.post(`/v1/tasks/${id}/confirm`, data)
export const rejectTask = (id, data) => http.post(`/v1/tasks/${id}/reject`, data)
export const taskLogs = (id, params) => http.get(`/v1/tasks/${id}/logs`, { params })
export const taskPatch = (id) => http.get(`/v1/tasks/${id}/patch`)

// ---------- 统计 ----------
export const statsOverview = (params) => http.get('/v1/stats/overview', { params })
export const statsTrend = (params) => http.get('/v1/stats/trend', { params })
export const statsGroup = (params) => http.get('/v1/stats/group', { params })

// ---------- 设置 ----------
export const getSettings = () => http.get('/v1/settings')
export const updateSettings = (data) => http.put('/v1/settings', data)

export function taskWSURL(taskId) {
  const base = import.meta.env.VITE_WS_BASE
  if (base) return `${base}/ws/tasks/${taskId}`
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${location.host}/ws/tasks/${taskId}`
}

export default http
