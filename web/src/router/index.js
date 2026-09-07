import { createRouter, createWebHashHistory } from 'vue-router'

const routes = [
  { path: '/', name: 'dashboard', component: () => import('@/views/Dashboard.vue'), meta: { title: '概览', icon: 'DataLine' } },
  { path: '/projects', name: 'projects', component: () => import('@/views/ProjectList.vue'), meta: { title: '项目管理', icon: 'Folder' } },
  { path: '/projects/:id', name: 'project-detail', component: () => import('@/views/ProjectDetail.vue'), meta: { title: '项目详情', hidden: true } },
  { path: '/repos', name: 'repos', component: () => import('@/views/RepoList.vue'), meta: { title: '仓库管理', icon: 'Coin' } },
  { path: '/rules', name: 'rules', component: () => import('@/views/RuleList.vue'), meta: { title: '修复规则', icon: 'Filter' } },
  { path: '/events', name: 'events', component: () => import('@/views/EventList.vue'), meta: { title: '事件中心', icon: 'Warning' } },
  { path: '/tasks', name: 'tasks', component: () => import('@/views/TaskList.vue'), meta: { title: '修复任务', icon: 'Tools' } },
  { path: '/tasks/:id', name: 'task-detail', component: () => import('@/views/TaskDetail.vue'), meta: { title: '任务详情', hidden: true } },
  { path: '/statistics', name: 'statistics', component: () => import('@/views/Statistics.vue'), meta: { title: '统计分析', icon: 'Histogram' } },
  { path: '/models', name: 'models', component: () => import('@/views/ModelConfig.vue'), meta: { title: '大模型配置', icon: 'Cpu' } },
  { path: '/credentials', name: 'credentials', component: () => import('@/views/CredentialList.vue'), meta: { title: '凭证中心', icon: 'Key' } },
  { path: '/tokens', name: 'tokens', component: () => import('@/views/TokenList.vue'), meta: { title: '项目令牌', icon: 'Ticket' } },
  { path: '/settings', name: 'settings', component: () => import('@/views/Settings.vue'), meta: { title: '运行设置', icon: 'Setting' } }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
  scrollBehavior: () => ({ top: 0 })
})

export const menus = routes.filter((r) => !r.meta?.hidden)

export default router
