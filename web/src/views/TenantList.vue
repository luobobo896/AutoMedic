<template>
  <div class="am-page">
    <div class="am-page-head">
      <div>
        <h2 class="am-page-title">租户管理</h2>
        <p class="am-page-desc">租户是数据隔离边界：项目、仓库、凭证、规则、事件、任务均归属某个租户。</p>
      </div>
      <el-button class="am-btn-soft" :icon="'Plus'" @click="openCreate">新增租户</el-button>
    </div>

    <div class="am-card-grid">
      <div v-for="t in tenants" :key="t.id" class="tenant-card">
        <div class="tenant-top">
          <span class="tenant-name">{{ t.name }}</span>
          <span class="am-pill am-mono">{{ t.key }}</span>
          <div class="am-flex-1" />
          <el-button class="am-icon-btn" link @click="edit(t)"><el-icon><EditPen /></el-icon></el-button>
          <el-button class="am-icon-btn am-icon-danger" link @click="remove(t)"><el-icon><Delete /></el-icon></el-button>
        </div>
        <div class="tenant-meta">
          <span class="am-pill">{{ t.user_count }} 用户</span>
          <span class="am-pill am-pill-accent">{{ t.project_count }} 项目</span>
          <span class="am-pill">{{ t.status === 'active' ? '启用中' : '已停用' }}</span>
        </div>
        <div v-if="t.remark" class="tenant-remark">{{ t.remark }}</div>
      </div>
      <div v-if="!tenants.length" class="am-card am-empty">暂无租户</div>
    </div>

    <el-dialog v-model="dialog" :title="form.id ? '编辑租户' : '新增租户'" width="520" class="am-dialog">
      <p class="am-dialog-desc">创建后会同步生成该租户的内置角色（租户管理员 / 开发 / 只读）。</p>
      <el-form :model="form" label-position="top" class="am-form">
        <div class="am-form-grid">
          <el-form-item label="租户名称"><el-input v-model="form.name" /></el-form-item>
          <el-form-item label="租户标识">
            <el-input v-model="form.key" :disabled="!!form.id" placeholder="英文标识，如 acme" />
          </el-form-item>
        </div>
        <el-form-item label="备注"><el-input v-model="form.remark" /></el-form-item>
        <template v-if="!form.id">
          <div class="am-form-divider">同时创建管理员（可选）</div>
          <div class="am-form-grid">
            <el-form-item label="管理员账号"><el-input v-model="form.admin_username" /></el-form-item>
            <el-form-item label="管理员密码"><el-input v-model="form.admin_password" type="password" show-password /></el-form-item>
          </div>
        </template>
        <el-form-item v-else label="状态">
          <el-select v-model="form.status" style="width:100%">
            <el-option label="启用" value="active" />
            <el-option label="停用" value="disabled" />
          </el-select>
        </el-form-item>
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
import { EditPen, Delete } from '@element-plus/icons-vue'
import { listTenants, createTenant, updateTenant, deleteTenant } from '@/api'

const tenants = ref([])
const dialog = ref(false)
const form = ref({})

async function load() {
  tenants.value = (await listTenants()).data || []
}

function openCreate() {
  form.value = { status: 'active' }
  dialog.value = true
}
function edit(t) {
  form.value = { id: t.id, name: t.name, key: t.key, remark: t.remark, status: t.status }
  dialog.value = true
}

async function submit() {
  const payload = {
    name: form.value.name, key: form.value.key, remark: form.value.remark || '',
    status: form.value.status || 'active', admin_username: form.value.admin_username,
    admin_password: form.value.admin_password
  }
  if (form.value.id) await updateTenant(form.value.id, { name: payload.name, remark: payload.remark, status: payload.status })
  else await createTenant(payload)
  ElMessage.success('已保存')
  dialog.value = false
  load()
}

async function remove(t) {
  const ok = await ElMessageBox.confirm(`确认删除租户「${t.name}」？租户下的用户与角色将一并删除。`, '警告', { type: 'warning' }).catch(() => false)
  if (!ok) return
  await deleteTenant(t.id)
  ElMessage.success('已删除')
  load()
}

onMounted(load)
</script>

<style scoped>
.am-card-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 14px; }
.tenant-card {
  background: var(--am-bg-elevated);
  border: 1px solid var(--am-border);
  border-radius: var(--am-radius);
  padding: 16px 18px;
}
.tenant-top { display: flex; align-items: center; gap: 8px; }
.tenant-name { font-size: 15px; font-weight: 600; }
.tenant-meta { margin-top: 12px; display: flex; gap: 6px; flex-wrap: wrap; }
.tenant-remark { margin-top: 10px; font-size: 12.5px; color: var(--am-text-dim); }
</style>
