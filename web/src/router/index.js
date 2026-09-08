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

const routes = [
  { path: '/', name: 'dashboard', component: Dashboard, meta: { title: '概览', icon: 'DataLine' } },
  { path: '/projects', name: 'projects', component: ProjectList, meta: { title: '项目管理', icon: 'Folder' } },
  { path: '/projects/:id', name: 'project-detail', component: ProjectDetail, meta: { title: '项目详情', hidden: true } },
  { path: '/repos', name: 'repos', component: RepoList, meta: { title: '仓库管理', icon: 'Coin' } },
  { path: '/rules', name: 'rules', component: RuleList, meta: { title: '修复规则', icon: 'Filter' } },
  { path: '/events', name: 'events', component: EventList, meta: { title: '事件中心', icon: 'Warning' } },
  { path: '/tasks', name: 'tasks', component: TaskList, meta: { title: '修复任务', icon: 'Tools' } },
  { path: '/tasks/:id', name: 'task-detail', component: TaskDetail, meta: { title: '任务详情', hidden: true } },
  { path: '/statistics', name: 'statistics', component: Statistics, meta: { title: '统计分析', icon: 'Histogram' } },
  { path: '/models', name: 'models', component: ModelConfig, meta: { title: '大模型配置', icon: 'Cpu' } },
  { path: '/credentials', name: 'credentials', component: CredentialList, meta: { title: '凭证中心', icon: 'Key' } },
  { path: '/tokens', name: 'tokens', component: TokenList, meta: { title: '项目令牌', icon: 'Ticket' } },
  { path: '/settings', name: 'settings', component: Settings, meta: { title: '运行设置', icon: 'Setting' } }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
  scrollBehavior: () => ({ top: 0 })
})

export const menus = routes.filter((r) => !r.meta?.hidden)

export default router
