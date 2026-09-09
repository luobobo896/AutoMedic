<template>
  <div class="am-page">
    <!-- 上手引导：首次进入默认展示，关掉后可从工具栏再次打开 -->
    <div class="am-card am-guide" v-if="showGuide">
      <div class="am-guide__head">
        <div>
          <div class="am-guide__title">四步接入：项目 → 仓库 → 规则 → 令牌</div>
          <div class="am-guide__sub">配完即可收告警并自动修复。未命中规则的告警会被忽略。</div>
        </div>
        <el-button link @click="hideGuide">不再显示</el-button>
      </div>
      <ol class="am-flow">
        <li v-for="(s, i) in flow" :key="s.title" class="am-flow__item">
          <span class="am-flow__no">{{ i + 1 }}</span>
          <span class="am-flow__title">{{ s.title }}</span>
        </li>
      </ol>
    </div>

    <div class="am-card">
      <div class="am-toolbar">
        <el-input v-model="query.keyword" class="am-search" placeholder="项目名称 / 标识" clearable @keyup.enter="search" @clear="search" />
        <el-button type="primary" :icon="'Plus'" @click="openCreate">新建项目</el-button>
        <div class="am-flex-1" />
        <el-button v-if="!showGuide" link @click="showGuide = true">使用引导</el-button>
        <el-button :icon="'Refresh'" aria-label="刷新" @click="load" />
      </div>

      <div v-loading="loading" class="am-projlist">
        <div v-if="!loading && !list.length" class="am-empty">
          <template v-if="!query.keyword">
            <el-icon :size="32"><FolderOpened /></el-icon>
            <div class="am-empty__title">还没有项目</div>
            <div class="am-empty__desc">先建项目，再挂仓库、规则和令牌。</div>
            <div class="am-empty__actions">
              <el-button type="primary" :icon="'Plus'" @click="openCreate">创建第一个项目</el-button>
            </div>
          </template>
          <template v-else>
            <div class="am-empty__title">没有匹配「{{ query.keyword }}」的项目</div>
            <div class="am-empty__desc">换个项目名称或标识再试试。</div>
            <div class="am-empty__actions">
              <el-button @click="clearKeyword">清空关键词</el-button>
            </div>
          </template>
        </div>

        <article v-for="row in list" :key="row.id" class="am-proj">
          <div class="am-proj__main">
            <button type="button" class="am-proj__name" @click="open(row)">{{ row.name }}</button>
            <span class="am-mono am-text-dim">{{ row.key }}</span>
            <div class="am-chips">
              <el-tag size="small" effect="plain" :type="ready(row) ? 'success' : 'warning'">
                {{ ready(row) ? '已就绪' : '待配置' }}
              </el-tag>
              <el-tag size="small" :type="row.fix_mode === 'auto' ? 'danger' : 'warning'" effect="plain">
                {{ row.fix_mode === 'auto' ? '全自动' : '半自动' }}
              </el-tag>
            </div>
          </div>
          <div class="am-chips am-proj__progress" aria-label="接入进度">
            <span class="am-chip" :class="{ 'is-ok': row.repo_count > 0 }">仓库 {{ row.repo_count || 0 }}</span>
            <span class="am-chip" :class="{ 'is-ok': row.rule_count > 0 }">规则 {{ row.rule_count || 0 }}</span>
            <span class="am-chip" :class="{ 'is-ok': row.token_count > 0 }">令牌 {{ row.token_count || 0 }}</span>
          </div>
          <div class="am-proj__ops">
            <el-switch
              v-model="row.enabled"
              size="small"
              aria-label="启用项目"
              @change="v => toggle(row, 'enabled', v)"
            />
            <el-button class="am-proj__enter" type="primary" @click="open(row)">进入配置</el-button>
            <el-dropdown trigger="click" @command="cmd => rowCommand(cmd, row)">
              <el-button class="am-proj__more" aria-label="更多操作">更多</el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="edit">编辑</el-dropdown-item>
                  <el-dropdown-item command="remove" divided>删除</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </article>
      </div>

      <el-pagination
        class="am-pagination"
        layout="total, sizes, prev, pager, next"
        :total="total" v-model:current-page="query.page" v-model:page-size="query.page_size"
        @current-change="load" @size-change="search"
      />
    </div>

    <el-dialog v-model="dialog" :title="form.id ? '编辑项目' : '新建项目'" class="am-dialog-form" width="640px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px">
        <el-form-item label="项目名称" prop="name">
          <el-input v-model="form.name" placeholder="例如：订单系统" maxlength="128" />
        </el-form-item>
        <el-form-item label="标识 Key" prop="key">
          <el-input v-model="form.key" placeholder="shop-api" :disabled="!!form.id" />
          <div class="am-field-help">英文、数字、- 或 _，全局唯一，创建后不可改。</div>
        </el-form-item>
        <el-form-item label="修复模式">
          <el-radio-group v-model="form.fix_mode" class="am-seg">
            <el-radio-button value="semi">半自动</el-radio-button>
            <el-radio-button value="auto">全自动</el-radio-button>
          </el-radio-group>
          <div class="am-field-help">半自动需人工确认后再推送；全自动直接提交并推送。</div>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
          <div class="am-field-help">关闭后不再自动修复该项目的告警。</div>
        </el-form-item>

        <el-collapse v-model="advanced">
          <el-collapse-item name="adv" title="高级选项">
            <el-form-item label="默认修复模型">
              <el-select v-model="form.default_model_id" clearable placeholder="留空则用全局默认模型" style="width:100%">
                <el-option v-for="m in models" :key="m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="默认审查模型">
              <el-select v-model="form.default_review_model_id" clearable placeholder="留空则与修复模型同一套" style="width:100%">
                <el-option v-for="m in models" :key="'r'+m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
              </el-select>
              <div class="am-field-help">还没有模型？先到「大模型配置」添加 Provider 与模型。</div>
            </el-form-item>
            <el-form-item label="业务上下文">
              <el-input v-model="form.context" type="textarea" :rows="4"
                placeholder="项目背景、核心链路、技术栈；dsh 用它理解业务语义、定位代码" />
            </el-form-item>
            <el-form-item label="描述"><el-input v-model="form.description" /></el-form-item>
          </el-collapse-item>
        </el-collapse>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { listProjects, createProject, updateProject, deleteProject, listModels } from '@/api'
import { ElMessageBox, ElMessage } from 'element-plus'

const router = useRouter()
const list = ref([])
const total = ref(0)
const loading = ref(false)
const saving = ref(false)
const dialog = ref(false)
const models = ref([])
const formRef = ref(null)
const advanced = ref([])
const query = reactive({ page: 1, page_size: 20, keyword: '' })
const form = ref({})

const GUIDE_KEY = 'am.guide.projects'
const showGuide = ref(localStorage.getItem(GUIDE_KEY) !== '1')
function hideGuide() {
  showGuide.value = false
  localStorage.setItem(GUIDE_KEY, '1')
}

const flow = [
  { title: '创建项目' },
  { title: '关联仓库' },
  { title: '配置规则' },
  { title: '生成令牌' }
]

const rules = {
  name: [{ required: true, message: '请填写项目名称', trigger: 'blur' }],
  key: [
    { required: true, message: '请填写项目标识', trigger: 'blur' },
    { pattern: /^[A-Za-z0-9._-]+$/, message: '只能是英文、数字、点、下划线或短横线', trigger: 'blur' }
  ]
}

function ready(row) {
  return row.repo_count > 0 && row.rule_count > 0 && row.token_count > 0
}

async function load() {
  loading.value = true
  try {
    const r = await listProjects(query)
    list.value = r.data?.list || []
    total.value = r.data?.total || 0
  } finally {
    loading.value = false
  }
}

// 筛选条件/每页条数变化后必须回到第 1 页，否则停留在旧页码可能拿到空列表
function search() {
  query.page = 1
  return load()
}

function clearKeyword() {
  query.keyword = ''
  return search()
}

function open(row) {
  router.push('/projects/' + row.id)
}

function rowCommand(cmd, row) {
  if (cmd === 'edit') openEdit(row)
  else if (cmd === 'remove') remove(row)
}

function openCreate() {
  form.value = { name: '', key: '', fix_mode: 'semi', enabled: true, default_model_id: null, default_review_model_id: null }
  advanced.value = []
  dialog.value = true
}

function openEdit(row) {
  form.value = { ...row }
  advanced.value = ['adv']
  dialog.value = true
}

function projectPayload(f) {
  return {
    name: f.name,
    key: f.key,
    description: f.description || '',
    fix_mode: f.fix_mode || 'semi',
    default_model_id: f.default_model_id || null,
    default_review_model_id: f.default_review_model_id || null,
    enabled: f.enabled !== false,
    context: f.context || ''
  }
}

async function submit() {
  const ok = await formRef.value.validate().catch(() => false)
  if (!ok) return
  if (saving.value) return
  saving.value = true
  const isEdit = !!form.value.id
  const payload = projectPayload(form.value)
  let newId = 0
  try {
    if (isEdit) {
      await updateProject(form.value.id, payload)
    } else {
      const r = await createProject(payload)
      newId = r?.data?.id || 0
    }
  } finally {
    saving.value = false
  }
  dialog.value = false
  await load()

  if (!isEdit) {
    ElMessage.success('项目已创建')
    if (!newId) return
    const go = await ElMessageBox.confirm(
      '下一步：关联代码仓库。没有仓库就无法改代码。',
      '创建成功',
      { confirmButtonText: '去关联仓库', cancelButtonText: '稍后再说', type: 'success' }
    ).then(() => true).catch(() => false)
    if (go) router.push('/projects/' + newId)
    return
  }
  ElMessage.success('已保存')
}

async function toggle(row, field, v) {
  await updateProject(row.id, { [field]: v })
  ElMessage.success('已更新')
}

async function remove(row) {
  const ok = await ElMessageBox.confirm(`确认删除项目「${row.name}」？其下仓库、规则与令牌会一并删除`, '警告', { type: 'warning' }).catch(() => false)
  if (!ok) return
  await deleteProject(row.id)
  ElMessage.success('已删除')
  load()
}

onMounted(async () => {
  const m = await listModels()
  models.value = m.data || []
  load()
})
</script>
