<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar">
        <el-input v-model="query.keyword" placeholder="项目名称 / 标识" clearable style="width:240px" @keyup.enter="search" @clear="search" />
        <el-button type="primary" :icon="'Plus'" @click="openCreate">新建项目</el-button>
        <div class="am-flex-1" />
        <el-button :icon="'Refresh'" @click="load" />
      </div>

      <el-table :data="list" v-loading="loading">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="name" label="项目名称" min-width="160" />
        <el-table-column prop="key" label="标识" width="130" />
        <el-table-column label="仓库数" width="90">
          <template #default="{ row }">{{ row.repo_count ?? 0 }}</template>
        </el-table-column>
        <el-table-column label="修复模式" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="row.fix_mode === 'auto' ? 'danger' : 'warning'" effect="plain">
              {{ row.fix_mode === 'auto' ? '全自动' : '半自动' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" size="small" @change="v => toggle(row, 'enabled', v)" />
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="170">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="230" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="$router.push('/projects/' + row.id)">详情</el-button>
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        style="margin-top:12px; justify-content:flex-end"
        layout="total, sizes, prev, pager, next"
        :total="total" v-model:current-page="query.page" v-model:page-size="query.page_size"
        @current-change="load" @size-change="search"
      />
    </div>

    <el-dialog v-model="dialog" :title="form.id ? '编辑项目' : '新建项目'" width="640">
      <el-form :model="form" label-width="110px">
        <el-form-item label="项目名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="标识 Key"><el-input v-model="form.key" placeholder="英文标识，如 shop-api" /></el-form-item>
        <el-form-item label="修复模式">
          <el-radio-group v-model="form.fix_mode">
            <el-radio value="semi">半自动（修复后人工确认）</el-radio>
            <el-radio value="auto">全自动（自动提交推送）</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="默认修复模型">
          <el-select v-model="form.default_model_id" clearable placeholder="留空则使用全局默认模型" style="width:100%">
            <el-option v-for="m in models" :key="m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="默认审查模型">
          <el-select v-model="form.default_review_model_id" clearable placeholder="留空则与修复模型同一套" style="width:100%">
            <el-option v-for="m in models" :key="'r'+m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="发布钩子">
          <el-input v-model="form.release_hook" placeholder="推送成功后在仓库目录执行，对应 git.release_hook；留空用系统设置" />
        </el-form-item>
        <el-form-item label="业务上下文">
          <el-input v-model="form.context" type="textarea" :rows="4"
            placeholder="写给 dsh 的项目背景，帮助它理解业务语义与定位代码" />
        </el-form-item>
        <el-form-item label="描述"><el-input v-model="form.description" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { listProjects, createProject, updateProject, deleteProject, listModels } from '@/api'
import { formatTime } from '@/utils/format'
import { ElMessageBox, ElMessage } from 'element-plus'

const list = ref([])
const total = ref(0)
const loading = ref(false)
const dialog = ref(false)
const models = ref([])
const query = reactive({ page: 1, page_size: 20, keyword: '' })
const form = ref({})

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

function openCreate() {
  form.value = { name: '', key: '', fix_mode: 'semi', enabled: true, default_model_id: null, default_review_model_id: null }
  dialog.value = true
}

function openEdit(row) {
  form.value = { ...row }
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
    release_hook: f.release_hook || '',
    enabled: f.enabled !== false,
    context: f.context || ''
  }
}

async function submit() {
  const payload = projectPayload(form.value)
  if (form.value.id) await updateProject(form.value.id, payload)
  else await createProject(payload)
  ElMessage.success('已保存')
  dialog.value = false
  load()
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
