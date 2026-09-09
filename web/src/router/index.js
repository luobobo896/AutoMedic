import { createRouter, createWebHashHistory } from 'vue-router'
import Dashboard from '@/views/Dashboard.vue'
import ProjectList from '@/views/ProjectList.vue'
import ProjectDetail from '@/views/ProjectDetail.vue'
import RepoList from '@/views/RepoList.vue'
import RuleList from '@/views/RuleList.vue'
import EventList from '@/views/EventList.vue'
import TaskList from '@/views/TaskList.vue'
import TaskDetail from '@/views/TaskDetail.vue'
import Statistics from '@/views/Statistics.vue'
import ModelConfig from '@/views/ModelConfig.vue'
import CredentialList from '@/views/CredentialList.vue'
import TokenList from '@/views/TokenList.vue'
import Settings from '@/views/Settings.vue'
import DictList from '@/views/DictList.vue'
import Login from '@/views/Login.vue'
import Forbidden from '@/views/Forbidden.vue'
import UserList from '@/views/UserList.vue'
import RoleList from '@/views/RoleList.vue'
import TenantList from '@/views/TenantList.vue'
import { getToken } from '@/api'
import { useAuth } from '@/store/auth'

const routes = [
  { path: '/login', name: 'login', component: Login, meta: { hidden: true, public: true } },
  { path: '/403', name: 'forbidden', component: Forbidden, meta: { hidden: true } },
  { path: '/', name: 'dashboard', component: Dashboard, meta: { title: '概览', icon: 'DataLine', perm: 'overview:read' } },
  { path: '/projects', name: 'projects', component: ProjectList, meta: { title: '项目管理', icon: 'Folder', perm: 'project:read' } },
  { path: '/projects/:id', name: 'project-detail', component: ProjectDetail, meta: { title: '项目详情', hidden: true, perm: 'project:read' } },
  { path: '/repos', name: 'repos', component: RepoList, meta: { title: '仓库管理', icon: 'Coin', perm: 'repo:read' } },
  { path: '/rules', name: 'rules', component: RuleList, meta: { title: '修复规则', icon: 'Filter', perm: 'rule:read' } },
  { path: '/events', name: 'events', component: EventList, meta: { title: '事件中心', icon: 'Warning', perm: 'event:read' } },
  { path: '/tasks', name: 'tasks', component: TaskList, meta: { title: '修复任务', icon: 'Tools', perm: 'task:read' } },
  { path: '/tasks/:id', name: 'task-detail', component: TaskDetail, meta: { title: '任务详情', hidden: true, perm: 'task:read' } },
  { path: '/statistics', name: 'statistics', component: Statistics, meta: { title: '统计分析', icon: 'Histogram', perm: 'stats:read' } },
  { path: '/models', name: 'models', component: ModelConfig, meta: { title: '大模型配置', icon: 'Cpu', perm: 'model:read' } },
  { path: '/credentials', name: 'credentials', component: CredentialList, meta: { title: '凭证中心', icon: 'Key', perm: 'credential:read' } },
  { path: '/tokens', name: 'tokens', component: TokenList, meta: { title: '项目令牌', icon: 'Ticket', perm: 'token:read' } },
  { path: '/settings', name: 'settings', component: Settings, meta: { title: '运行设置', icon: 'Setting', perm: 'settings:read' } },
  { path: '/dicts', name: 'dicts', component: DictList, meta: { title: '选项字典', icon: 'Collection', perm: 'settings:read' } },
  { path: '/users', name: 'users', component: UserList, meta: { title: '用户管理', icon: 'User', perm: 'user:read' } },
  { path: '/roles', name: 'roles', component: RoleList, meta: { title: '角色权限', icon: 'Avatar', perm: 'role:read' } },
  { path: '/tenants', name: 'tenants', component: TenantList, meta: { title: '租户管理', icon: 'OfficeBuilding', perm: 'tenant:read' } }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
  scrollBehavior: () => ({ top: 0 })
})

// 当前用户第一个有权限的落地路由；一个都没有则返回 '/403'
// 不能固定回退到 '/'（它自身要求 overview:read），否则会触发无限重定向
function landingPath() {
  const { can } = useAuth()
  const hit = routes.find((r) => {
    if (r.path === '/login' || r.path === '/403') return false
    if (r.path.includes(':')) return false
    return !r.meta?.perm || can(r.meta.perm)
  })
  return hit ? hit.path : '/403'
}

router.beforeEach((to) => {
  const { can } = useAuth()
  if (!to.meta?.public && !getToken()) {
    return { path: '/login', query: to.path === '/' ? {} : { redirect: to.fullPath } }
  }
  if (to.path === '/login' && getToken()) {
    return { path: landingPath() }
  }
  if (to.meta?.perm && !can(to.meta.perm)) {
    const target = landingPath()
    return { path: target === to.path ? '/403' : target }
  }
  return true
})

// 侧边栏菜单：按权限过滤
export function visibleMenus() {
  const { can } = useAuth()
  return routes.filter((r) => !r.meta?.hidden && (!r.meta?.perm || can(r.meta.perm)))
}

export default router
