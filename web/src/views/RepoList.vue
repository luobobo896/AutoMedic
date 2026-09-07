<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar">
        <el-select v-model="query.project_id" clearable placeholder="全部项目" style="width:200px" @change="load">
          <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
        <el-button type="primary" :icon="'Plus'" @click="openCreate">关联仓库</el-button>
        <div class="am-flex-1" />
        <el-button :icon="'Refresh'" @click="load" />
      </div>

      <el-table :data="list" v-loading="loading" size="small">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="名称" width="150" />
        <el-table-column label="所属项目" width="150">
          <template #default="{ row }">{{ projectName(row.project_id) }}</template>
        </el-table-column>
        <el-table-column prop="url" label="地址" min-width="240" show-overflow-tooltip />
        <el-table-column prop="branch" label="分支" width="100" />
        <el-table-column prop="language" label="语言" width="90" />
        <el-table-column label="凭证" width="130">
          <template #default="{ row }">{{ row.credential?.name || '未配置' }}</template>
        </el-table-column>
        <el-table-column label="模型" width="150">
          <template #default="{ row }">{{ row.model?.name || '继承项目' }}</template>
        </el-table-column>
        <el-table-column label="自动推送" width="90">
          <template #default="{ row }">{{ row.auto_push ? '是' : '否' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="260" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openTree(row)">目录树</el-button>
            <el-button link type="primary" @click="testConn(row)">测试</el-button>
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="dialog" :title="form.id ? '编辑仓库' : '关联仓库'" width="620">
      <el-form :model="form" label-width="110px">
        <el-form-item label="所属项目">
          <el-select v-model="form.project_id" style="width:100%">
            <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="仓库名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="仓库地址"><el-input v-model="form.url" placeholder="git@github.com:org/repo.git 或 https://..." /></el-form-item>
        <el-form-item label="分支"><el-input v-model="form.branch" /></el-form-item>
        <el-form-item label="主要语言"><el-input v-model="form.language" /></el-form-item>
        <el-form-item label="关注路径"><el-input v-model="form.code_paths" placeholder="逗号分隔" /></el-form-item>
        <el-form-item label="git 凭证">
          <el-select v-model="form.credential_id" clearable style="width:100%">
            <el-option v-for="c in credentials" :key="c.id" :label="`${c.name}（${c.type}）`" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="模型">
          <el-select v-model="form.model_id" clearable style="width:100%" placeholder="留空继承项目">
            <el-option v-for="m in models" :key="m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="自动推送"><el-switch v-model="form.auto_push" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" @click="submit">保存</el-button>
      </template>
    </el-dialog>

    <RepoTreeDrawer v-model="treeDrawer" :repo="treeRepo" />
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { listRepos, createRepo, updateRepo, deleteRepo, testRepo, listProjects, listCredentials, listModels } from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import RepoTreeDrawer from './RepoTreeDrawer.vue'

const list = ref([])
const projects = ref([])
const credentials = ref([])
const models = ref([])
const loading = ref(false)
const dialog = ref(false)
const treeDrawer = ref(false)
const treeRepo = ref(null)
const query = reactive({ project_id: '' })
const form = ref({ auto_push: true, enabled: true, branch: 'main' })

function projectName(id) {
  return projects.value.find(p => p.id === id)?.name || '-'
}

async function load() {
  loading.value = true
  try {
    const r = await listRepos(query.project_id ? { project_id: query.project_id } : {})
    list.value = r.data?.list || []
  } finally { loading.value = false }
}

function openCreate() {
  form.value = { auto_push: true, enabled: true, branch: 'main', project_id: query.project_id || projects.value[0]?.id }
  dialog.value = true
}
function openEdit(row) { form.value = { ...row }; dialog.value = true }

async function submit() {
  if (form.value.id) await updateRepo(form.value.id, form.value)
  else await createRepo(form.value)
  ElMessage.success('已保存')
  dialog.value = false
  load()
}

function openTree(row) {
  treeRepo.value = row
  treeDrawer.value = true
}

async function testConn(row) {
  await testRepo(row.id)
  ElMessage.success('连通性正常')
}

async function remove(row) {
  await ElMessageBox.confirm(`确认删除仓库「${row.name}」？`, '警告', { type: 'warning' })
  await deleteRepo(row.id)
  ElMessage.success('已删除')
  load()
}

onMounted(async () => {
  const [p, c, m] = await Promise.all([listProjects({ page_size: 100 }), listCredentials(), listModels()])
  projects.value = p.data?.list || []
  credentials.value = c.data || []
  models.value = m.data || []
  load()
})
</script>
