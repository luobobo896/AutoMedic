<template>
  <div class="am-page" v-loading="loading">
    <div class="am-toolbar">
      <el-button link type="primary" @click="$router.push('/projects')">← 返回项目列表</el-button>
      <h3 style="margin:0">{{ detail.project?.name }}</h3>
      <el-tag size="small">{{ detail.project?.key }}</el-tag>
      <el-tag size="small" :type="detail.project?.fix_mode === 'auto' ? 'danger' : 'warning'" effect="plain">
        {{ detail.project?.fix_mode === 'auto' ? '全自动' : '半自动确认' }}
      </el-tag>
      <div class="am-flex-1" />
      <el-button :icon="'Refresh'" @click="load" />
    </div>

    <el-tabs v-model="tab">
      <el-tab-pane label="仓库" name="repos">
        <div class="am-card">
          <div class="am-toolbar">
            <el-button type="primary" size="small" :icon="'Plus'" @click="openCreateRepo">关联仓库</el-button>
          </div>
          <el-table :data="detail.repos || []" size="small">
            <el-table-column prop="name" label="名称" width="160" />
            <el-table-column prop="url" label="地址" min-width="240" show-overflow-tooltip />
            <el-table-column prop="branch" label="分支" width="110" />
            <el-table-column prop="language" label="语言" width="90" />
            <el-table-column label="凭证" width="150">
              <template #default="{ row }">{{ row.credential?.name || '未配置' }}</template>
            </el-table-column>
            <el-table-column label="修复模型" width="120">
              <template #default="{ row }">{{ row.model?.name || '继承项目' }}</template>
            </el-table-column>
            <el-table-column label="审查模型" width="120">
              <template #default="{ row }">{{ row.review_model?.name || '同修复' }}</template>
            </el-table-column>
            <el-table-column label="自动推送" width="90">
              <template #default="{ row }">{{ row.auto_push ? '是' : '否' }}</template>
            </el-table-column>
            <el-table-column label="操作" width="260">
              <template #default="{ row }">
                <el-button link type="primary" @click="openTree(row)">目录树</el-button>
                <el-button link type="primary" @click="openReview(row)">审查</el-button>
                <el-button link type="primary" @click="testConn(row)">测试</el-button>
                <el-button link type="danger" @click="removeRepo(row)">移除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>

      <el-tab-pane label="修复规则" name="rules">
        <div class="am-card">
          <div class="am-toolbar">
            <el-button type="primary" size="small" :icon="'Plus'" @click="openCreateRule">新增规则</el-button>
            <span class="am-text-dim" style="font-size:12px">规则决定告警是否触发代码修改；未命中任何规则的事件默认忽略</span>
          </div>
          <el-table :data="detail.rules || []" size="small">
            <el-table-column prop="name" label="规则名称" width="180" />
            <el-table-column prop="priority" label="优先级" width="80" />
            <el-table-column label="级别" width="120">
              <template #default="{ row }">{{ row.levels || '不限' }}</template>
            </el-table-column>
            <el-table-column label="命中关键字" min-width="200" show-overflow-tooltip>
              <template #default="{ row }">{{ row.keywords || '-' }}</template>
            </el-table-column>
            <el-table-column label="排除关键字" min-width="200" show-overflow-tooltip>
              <template #default="{ row }">{{ row.exclude_keywords || '-' }}</template>
            </el-table-column>
            <el-table-column label="动作" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="row.action === 'fix' ? 'success' : 'info'">
                  {{ row.action === 'fix' ? '修复' : '忽略' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="80">
              <template #default="{ row }">{{ row.enabled ? '是' : '否' }}</template>
            </el-table-column>
            <el-table-column label="操作" width="140">
              <template #default="{ row }">
                <el-button link type="primary" @click="editRule(row)">编辑</el-button>
                <el-button link type="danger" @click="removeRule(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>

      <el-tab-pane label="投递令牌" name="tokens">
        <div class="am-card">
          <div class="am-toolbar">
            <el-button type="primary" size="small" :icon="'Plus'" @click="createProjectToken">新建令牌</el-button>
          </div>
          <el-table :data="detail.tokens || []" size="small">
            <el-table-column prop="name" label="名称" width="180" />
            <el-table-column prop="prefix" label="前缀" width="140" />
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '禁用' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="最近使用" width="170">
              <template #default="{ row }">{{ formatTime(row.last_used_at) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="180">
              <template #default="{ row }">
                <el-button link type="primary" @click="toggleToken(row)">{{ row.enabled ? '禁用' : '启用' }}</el-button>
                <el-button link type="danger" @click="removeToken(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>

      <el-tab-pane label="项目设置" name="settings">
        <div class="am-card">
          <el-form :model="form" label-width="120px">
            <el-form-item label="项目名称"><el-input v-model="form.name" /></el-form-item>
            <el-form-item label="修复模式">
              <el-radio-group v-model="form.fix_mode">
                <el-radio value="semi">半自动（修复后人工确认再提交）</el-radio>
                <el-radio value="auto">全自动（自动提交并推送）</el-radio>
              </el-radio-group>
            </el-form-item>
            <el-form-item label="默认修复模型">
              <el-select v-model="form.default_model_id" clearable style="width:320px" placeholder="留空用全局默认">
                <el-option v-for="m in models" :key="m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="默认审查模型">
              <el-select v-model="form.default_review_model_id" clearable style="width:320px" placeholder="留空则与修复模型同一套">
                <el-option v-for="m in models" :key="'r'+m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="发布钩子">
              <el-input v-model="form.release_hook" placeholder="推送成功后在仓库目录执行的命令，对应 git.release_hook；留空则用系统设置" />
              <div class="am-text-dim" style="font-size:12px;margin-top:4px">
                「发布」不是独立流水线，而是推送成功后执行的可配置命令（项目级覆盖系统设置 `git.release_hook`）。
              </div>
            </el-form-item>
            <el-form-item label="业务上下文">
              <el-input v-model="form.context" type="textarea" :rows="6"
                placeholder="项目的业务背景、核心链路、技术栈，dsh 会用这些信息定位问题" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveProject">保存</el-button>
            </el-form-item>
          </el-form>
        </div>
      </el-tab-pane>
    </el-tabs>

    <!-- 仓库对话框 -->
    <el-dialog v-model="repoDialog" title="关联仓库" width="620">
      <el-form :model="repoForm" label-width="110px">
        <el-form-item label="仓库名称"><el-input v-model="repoForm.name" /></el-form-item>
        <el-form-item label="仓库地址"><el-input v-model="repoForm.url" placeholder="git@github.com:org/repo.git 或 https://..." /></el-form-item>
        <el-form-item label="分支">
          <el-select v-model="repoForm.branch" filterable allow-create default-first-option style="width:100%" placeholder="main">
            <el-option v-for="b in dict.values('git_branch')" :key="b" :label="b" :value="b" />
          </el-select>
        </el-form-item>
        <el-form-item label="主要语言">
          <el-select v-model="repoForm.language" filterable allow-create default-first-option clearable style="width:100%">
            <el-option v-for="l in dict.values('language')" :key="l" :label="l" :value="l" />
          </el-select>
        </el-form-item>
        <el-form-item label="关注路径">
          <el-select v-model="repoForm.code_paths" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%" placeholder="勾选目录前缀，可输入后回车">
            <el-option v-for="p in dict.values('code_path')" :key="p" :label="p" :value="p" />
          </el-select>
        </el-form-item>
        <el-form-item label="git 凭证">
          <el-select v-model="repoForm.credential_id" clearable style="width:100%">
            <el-option v-for="c in credentials" :key="c.id" :label="`${c.name}（${c.type}）`" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="修复模型">
          <el-select v-model="repoForm.model_id" clearable style="width:100%" placeholder="留空继承项目配置">
            <el-option v-for="m in models" :key="m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="审查模型">
          <el-select v-model="repoForm.review_model_id" clearable style="width:100%" placeholder="留空则与修复模型同一套">
            <el-option v-for="m in models" :key="'r'+m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="自动推送"><el-switch v-model="repoForm.auto_push" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="repoDialog = false">取消</el-button>
        <el-button type="primary" @click="saveRepo">保存</el-button>
      </template>
    </el-dialog>

    <!-- 规则对话框 -->
    <el-dialog v-model="ruleDialog" :title="ruleForm.id ? '编辑规则' : '新增规则'" width="720">
      <el-form :model="ruleForm" label-width="120px">
        <el-form-item label="规则名称"><el-input v-model="ruleForm.name" /></el-form-item>
        <el-form-item label="优先级"><el-input-number v-model="ruleForm.priority" :min="0" :max="999" /></el-form-item>
        <el-form-item label="日志级别">
          <el-select v-model="ruleForm.levels" multiple clearable collapse-tags style="width:100%" placeholder="留空不限">
            <el-option v-for="o in dict.options('log_level')" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="命中关键字">
          <el-select v-model="ruleForm.keywords" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%" placeholder="勾选或输入后回车">
            <el-option v-for="k in dict.values('hit_keyword')" :key="k" :label="k" :value="k" />
          </el-select>
        </el-form-item>
        <el-form-item label="排除关键字">
          <el-select v-model="ruleForm.exclude_keywords" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%">
            <el-option v-for="k in dict.values('exclude_keyword')" :key="k" :label="k" :value="k" />
          </el-select>
        </el-form-item>
        <el-form-item label="来源白名单">
          <el-select v-model="ruleForm.sources" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%" placeholder="留空不限">
            <el-option v-for="s in dict.values('event_source')" :key="s" :label="s" :value="s" />
          </el-select>
        </el-form-item>
        <el-form-item label="排除来源">
          <el-select v-model="ruleForm.exclude_sources" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%">
            <el-option v-for="s in dict.values('event_source')" :key="'ex-'+s" :label="s" :value="s" />
          </el-select>
        </el-form-item>
        <el-form-item label="频次阈值">
          <el-input-number v-model="ruleForm.min_count" :min="1" /> 次 /
          <el-select v-model="ruleForm.window_sec" style="width:140px">
            <el-option v-for="o in dict.numberOptions('window_sec')" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="冷却时间">
          <el-select v-model="ruleForm.cooldown_sec" style="width:160px">
            <el-option v-for="o in dict.numberOptions('cooldown_sec')" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="动作">
          <el-radio-group v-model="ruleForm.action">
            <el-radio value="fix">触发修复</el-radio>
            <el-radio value="ignore">忽略</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="修复模式">
          <el-radio-group v-model="ruleForm.fix_mode">
            <el-radio value="">继承项目</el-radio>
            <el-radio value="auto">全自动</el-radio>
            <el-radio value="semi">半自动</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="附加要求">
          <el-input v-model="ruleForm.prompt_template" type="textarea" :rows="3"
            placeholder="追加到 dsh 任务的额外约束，如：必须补充单元测试" />
        </el-form-item>
        <el-form-item label="最大重试"><el-input-number v-model="ruleForm.max_retries" :min="0" :max="10" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="ruleForm.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ruleDialog = false">取消</el-button>
        <el-button type="primary" @click="saveRule">保存</el-button>
      </template>
    </el-dialog>

    <RepoTreeDrawer v-model="treeDrawer" :repo="treeRepo" />
    <RepoReviewDrawer v-model="reviewDrawer" :repo="reviewRepo" />
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  getProject, updateProject, createRepo, deleteRepo, testRepo,
  createRule, updateRule, deleteRule, createToken, updateToken, deleteToken,
  listModels, listCredentials
} from '@/api'
import { formatTime } from '@/utils/format'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useDicts, splitCSV, joinCSV } from '@/composables/useDicts'
import RepoTreeDrawer from './RepoTreeDrawer.vue'
import RepoReviewDrawer from './RepoReviewDrawer.vue'

const dict = useDicts()
const route = useRoute()
const id = ref(route.params.id)
const detail = ref({})
const loading = ref(false)
const tab = ref('repos')
const models = ref([])
const credentials = ref([])

const repoDialog = ref(false)
const treeDrawer = ref(false)
const treeRepo = ref(null)
const reviewDrawer = ref(false)
const reviewRepo = ref(null)
const ruleDialog = ref(false)
const repoForm = ref({ auto_push: true, branch: 'main', code_paths: [] })
const ruleForm = ref({
  enabled: true, action: 'fix', priority: 0, min_count: 1, window_sec: 300, cooldown_sec: 600,
  max_retries: 2, fix_mode: '', levels: ['fatal', 'error'], keywords: [], exclude_keywords: [],
  sources: [], exclude_sources: []
})
const form = ref({})

async function load() {
  loading.value = true
  try {
    const r = await getProject(id.value)
    detail.value = r.data || {}
    form.value = { ...(detail.value.project || {}) }
  } finally {
    loading.value = false
  }
}

async function saveRepo() {
  const f = repoForm.value
  await createRepo({
    project_id: Number(id.value), name: f.name, url: f.url, branch: f.branch || 'main',
    language: f.language || '', code_paths: Array.isArray(f.code_paths) ? joinCSV(f.code_paths) : (f.code_paths || ''),
    credential_id: f.credential_id || null, model_id: f.model_id || null,
    review_model_id: f.review_model_id || null, auto_push: f.auto_push !== false
  })
  ElMessage.success('已关联')
  repoDialog.value = false
  load()
}

function openTree(row) {
  treeRepo.value = row
  treeDrawer.value = true
}
function openReview(row) {
  reviewRepo.value = row
  reviewDrawer.value = true
}

async function testConn(row) {
  try {
    await testRepo(row.id)
    ElMessage.success('连通性正常')
  } catch { /* 错误已在拦截器提示 */ }
}

async function removeRepo(row) {
  await ElMessageBox.confirm(`确认移除仓库「${row.name}」？`, '警告', { type: 'warning' })
  await deleteRepo(row.id)
  load()
}

function openCreateRepo() {
  repoForm.value = { auto_push: true, branch: 'main', code_paths: [], language: '' }
  repoDialog.value = true
}

function openCreateRule() {
  ruleForm.value = {
    enabled: true, action: 'fix', priority: 0, min_count: 1, window_sec: 300, cooldown_sec: 600,
    max_retries: 2, fix_mode: '', levels: ['fatal', 'error'], keywords: [], exclude_keywords: [],
    sources: [], exclude_sources: []
  }
  ruleDialog.value = true
}

function editRule(row) {
  ruleForm.value = {
    id: row.id, name: row.name, enabled: row.enabled, priority: row.priority,
    levels: splitCSV(row.levels), sources: splitCSV(row.sources), keywords: splitCSV(row.keywords),
    exclude_keywords: splitCSV(row.exclude_keywords), exclude_sources: splitCSV(row.exclude_sources),
    min_count: row.min_count, window_sec: row.window_sec, cooldown_sec: row.cooldown_sec,
    action: row.action, repo_ids: row.repo_ids, model_id: row.model_id, fix_mode: row.fix_mode || '',
    max_retries: row.max_retries, prompt_template: row.prompt_template, description: row.description
  }
  ruleDialog.value = true
}

async function saveRule() {
  const f = ruleForm.value
  const payload = {
    project_id: Number(id.value), name: f.name, enabled: f.enabled !== false, priority: f.priority || 0,
    levels: joinCSV(f.levels), sources: joinCSV(f.sources), keywords: joinCSV(f.keywords),
    all_keywords: '', exclude_keywords: joinCSV(f.exclude_keywords),
    exclude_sources: joinCSV(f.exclude_sources), pattern: '',
    min_count: f.min_count, window_sec: f.window_sec, cooldown_sec: f.cooldown_sec,
    action: f.action || 'fix', repo_ids: f.repo_ids || '', model_id: f.model_id || null,
    fix_mode: f.fix_mode || '', max_retries: f.max_retries,
    prompt_template: f.prompt_template || '', description: f.description || ''
  }
  if (f.id) await updateRule(f.id, payload)
  else await createRule(payload)
  ElMessage.success('已保存')
  ruleDialog.value = false
  load()
}

async function removeRule(row) {
  await ElMessageBox.confirm(`确认删除规则「${row.name}」？`, '警告', { type: 'warning' })
  await deleteRule(row.id)
  load()
}

async function createProjectToken() {
  const r = await createToken({ project_id: Number(id.value), name: 'collector-' + Date.now().toString().slice(-6) })
  await ElMessageBox.alert(
    `请在日志采集器中配置该令牌，它只会显示一次：\n\n${r.data.plain_token}`,
    '令牌已创建', { confirmButtonText: '我已保存' }
  )
  load()
}

async function toggleToken(row) {
  await updateToken(row.id, { enabled: !row.enabled })
  load()
}

async function removeToken(row) {
  await ElMessageBox.confirm('确认删除该令牌？', '警告', { type: 'warning' })
  await deleteToken(row.id)
  load()
}

async function saveProject() {
  const f = form.value
  await updateProject(id.value, {
    name: f.name, description: f.description || '', fix_mode: f.fix_mode,
    default_model_id: f.default_model_id || null,
    default_review_model_id: f.default_review_model_id || null,
    release_hook: f.release_hook || '', enabled: f.enabled !== false, context: f.context || ''
  })
  ElMessage.success('已保存')
  load()
}

watch(() => route.params.id, (v) => { id.value = v; load() })

onMounted(async () => {
  const [m, c] = await Promise.all([listModels(), listCredentials(), dict.load()])
  models.value = m.data || []
  credentials.value = c.data || []
  load()
})
</script>
