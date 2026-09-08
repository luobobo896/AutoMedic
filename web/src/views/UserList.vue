<template>
  <div class="am-page">
    <div class="am-page-head">
      <div>
        <h2 class="am-page-title">用户管理</h2>
        <p class="am-page-desc">按租户维护账号与角色；平台超管可跨租户创建用户。</p>
      </div>
      <el-button class="am-btn-soft" :icon="'Plus'" @click="openCreate">新增用户</el-button>
    </div>

    <div class="am-card">
      <el-table :data="users" v-loading="loading" size="small">
        <el-table-column prop="username" label="账号" width="160" />
        <el-table-column prop="display_name" label="姓名" width="160" />
        <el-table-column label="角色" min-width="220">
          <template #default="{ row }">
            <span v-for="r in row.roles" :key="r.id" class="am-pill am-pill-accent" style="margin-right:6px">{{ r.name || r.code }}</span>
            <span v-if="!row.roles?.length" class="am-text-dim">未分配</span>
          </template>
        </el-table-column>
        <el-table-column label="租户" width="140">
          <template #default="{ row }">{{ row.tenant?.name || ('#' + row.tenant_id) }}</template>
        </el-table-column>
        <el-table-column label="类型" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="row.is_super ? 'warning' : 'info'" effect="plain">
              {{ row.is_super ? '平台超管' : '租户用户' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="row.status === 'active' ? 'success' : 'info'" effect="plain">
              {{ row.status === 'active' ? '启用' : '停用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="edit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="dialog" :title="form.id ? '编辑用户' : '新增用户'" width="520" class="am-dialog">
      <p class="am-dialog-desc">角色决定权限范围；未分配角色时仅有登录权限。</p>
      <el-form :model="form" label-position="top" class="am-form">
        <div class="am-form-grid">
          <el-form-item label="账号"><el-input v-model="form.username" :disabled="!!form.id" /></el-form-item>
          <el-form-item label="姓名"><el-input v-model="form.display_name" /></el-form-item>
        </div>
        <div class="am-form-grid">
          <el-form-item label="密码">
            <el-input v-model="form.password" type="password" show-password
              :placeholder="form.id ? '留空则不修改' : '至少 6 位'" />
          </el-form-item>
          <el-form-item label="邮箱"><el-input v-model="form.email" /></el-form-item>
        </div>
        <el-form-item v-if="isSuper" label="所属租户">
          <el-select v-model="form.tenant_id" style="width:100%">
            <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="角色">
          <el-select v-model="form.role_ids" multiple style="width:100%">
            <el-option v-for="r in roles" :key="r.id" :label="r.name" :value="r.id" />
          </el-select>
        </el-form-item>
        <div class="am-form-grid">
          <el-form-item label="状态">
            <el-select v-model="form.status" style="width:100%">
              <el-option label="启用" value="active" />
              <el-option label="停用" value="disabled" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="isSuper" label="平台超管">
            <el-switch v-model="form.is_super" />
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button class="am-btn-soft" @click="dialog = false">取消</el-button>
        <el-button type="primary" class="am-btn-primary" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { listUsers, createUser, updateUser, deleteUser, listRoles, listTenants } from '@/api'
import { useAuth } from '@/store/auth'

const { isSuper, state } = useAuth()
const users = ref([])
const roles = ref([])
const tenants = ref([])
const loading = ref(false)
const dialog = ref(false)
const form = ref({ status: 'active', role_ids: [] })

async function load() {
  loading.value = true
  try {
    const [u, r] = await Promise.all([listUsers(), listRoles()])
    users.value = u.data?.list || u.data || []
    roles.value = r.data || []
    if (isSuper.value) tenants.value = (await listTenants()).data || []
  } finally {
    loading.value = false
  }
}

function openCreate() {
  form.value = { status: 'active', role_ids: [], tenant_id: state.user?.tenant_id || 0 }
  dialog.value = true
}
function edit(row) {
  form.value = {
    id: row.id, username: row.username, display_name: row.display_name, email: row.email,
    tenant_id: row.tenant_id, status: row.status, is_super: row.is_super,
    role_ids: (row.roles || []).map(r => r.id)
  }
  dialog.value = true
}

async function submit() {
  const payload = {
    username: form.value.username, display_name: form.value.display_name, email: form.value.email,
    tenant_id: form.value.tenant_id, status: form.value.status, is_super: !!form.value.is_super,
    role_ids: form.value.role_ids || []
  }
  if (form.value.password) payload.password = form.value.password
  if (form.value.id) await updateUser(form.value.id, payload)
  else await createUser(payload)
  ElMessage.success('已保存')
  dialog.value = false
  load()
}

async function remove(row) {
  await ElMessageBox.confirm(`确认删除用户「${row.username}」？`, '警告', { type: 'warning' })
  await deleteUser(row.id)
  ElMessage.success('已删除')
  load()
}

onMounted(load)
</script>
