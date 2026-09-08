<template>
  <div class="am-page">
    <div class="am-page-head">
      <div>
        <h2 class="am-page-title">角色与权限</h2>
        <p class="am-page-desc">基于 RBAC：用户绑定角色，角色持有权限码。内置角色不可删除。</p>
      </div>
      <el-button class="am-btn-soft" :icon="'Plus'" @click="openCreate">新增角色</el-button>
    </div>

    <div class="am-card-grid">
      <div v-for="r in roles" :key="r.id" class="role-card">
        <div class="role-top">
          <div>
            <div class="role-name">{{ r.name }}</div>
            <div class="role-code am-mono">{{ r.code }}</div>
          </div>
          <div class="am-flex-1" />
          <span v-if="r.builtin" class="am-pill">内置</span>
          <el-button class="am-icon-btn" link :disabled="r.builtin" @click="edit(r)">
            <el-icon><EditPen /></el-icon>
          </el-button>
          <el-button class="am-icon-btn am-icon-danger" link :disabled="r.builtin" @click="remove(r)">
            <el-icon><Delete /></el-icon>
          </el-button>
        </div>
        <div class="role-perms">
          <span v-for="p in r.permissions" :key="p" class="am-pill am-pill-perm">{{ permName(p) }}</span>
          <span v-if="!r.permissions?.length" class="am-text-dim">无权限</span>
        </div>
      </div>
      <div v-if="!roles.length" class="am-card am-empty">暂无角色</div>
    </div>

    <el-dialog v-model="dialog" :title="form.id ? '编辑角色' : '新增角色'" width="640" class="am-dialog">
      <p class="am-dialog-desc">按分组勾选权限；平台超管自动拥有全部权限。</p>
      <el-form :model="form" label-position="top" class="am-form">
        <div class="am-form-grid">
          <el-form-item label="角色名称"><el-input v-model="form.name" /></el-form-item>
          <el-form-item label="角色编码">
            <el-input v-model="form.code" :disabled="!!form.id" placeholder="如 ops" />
          </el-form-item>
        </div>
        <el-form-item label="权限">
          <div class="perm-groups">
            <div v-for="(items, group) in grouped" :key="group" class="perm-group">
              <div class="perm-group-title">
                {{ group }}
                <el-button link size="small" @click="toggleGroup(items)">全选/反选</el-button>
              </div>
              <el-checkbox-group v-model="form.permissions">
                <el-checkbox v-for="p in items" :key="p.code" :value="p.code" :label="p.code">
                  {{ p.name }}
                </el-checkbox>
              </el-checkbox-group>
            </div>
          </div>
        </el-form-item>
        <el-form-item label="备注"><el-input v-model="form.remark" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button class="am-btn-soft" @click="dialog = false">取消</el-button>
        <el-button type="primary" class="am-btn-primary" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { EditPen, Delete } from '@element-plus/icons-vue'
import { listRoles, createRole, updateRole, deleteRole, listPermissions } from '@/api'

const roles = ref([])
const catalog = ref([])
const dialog = ref(false)
const form = ref({ permissions: [] })

const grouped = computed(() => {
  const m = {}
  for (const p of catalog.value) {
    ;(m[p.group] = m[p.group] || []).push(p)
  }
  return m
})

function permName(code) {
  return catalog.value.find(p => p.code === code)?.name || code
}

async function load() {
  const [r, c] = await Promise.all([listRoles(), listPermissions()])
  roles.value = r.data || []
  catalog.value = c.data || []
}

function openCreate() {
  form.value = { permissions: [] }
  dialog.value = true
}
function edit(r) {
  form.value = { id: r.id, name: r.name, code: r.code, remark: r.remark, permissions: [...(r.permissions || [])] }
  dialog.value = true
}

function toggleGroup(items) {
  const codes = items.map(i => i.code)
  const all = codes.every(c => form.value.permissions.includes(c))
  form.value.permissions = all
    ? form.value.permissions.filter(c => !codes.includes(c))
    : [...new Set([...form.value.permissions, ...codes])]
}

async function submit() {
  if (form.value.id) await updateRole(form.value.id, form.value)
  else await createRole(form.value)
  ElMessage.success('已保存')
  dialog.value = false
  load()
}

async function remove(r) {
  await ElMessageBox.confirm(`确认删除角色「${r.name}」？`, '警告', { type: 'warning' })
  await deleteRole(r.id)
  ElMessage.success('已删除')
  load()
}

onMounted(load)
</script>

<style scoped>
.am-card-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(340px, 1fr)); gap: 14px; }
.role-card {
  background: var(--am-bg-elevated);
  border: 1px solid var(--am-border);
  border-radius: var(--am-radius);
  padding: 16px 18px;
}
.role-top { display: flex; align-items: flex-start; gap: 8px; }
.role-name { font-size: 15px; font-weight: 600; }
.role-code { font-size: 12px; color: var(--am-text-dim); margin-top: 2px; }
.role-perms { margin-top: 12px; display: flex; flex-wrap: wrap; gap: 6px; }
.am-pill-perm { background: var(--am-primary-soft); color: var(--am-primary); }
.perm-groups { max-height: 46vh; overflow-y: auto; width: 100%; padding-right: 6px; }
.perm-group { margin-bottom: 14px; }
.perm-group-title {
  display: flex; align-items: center; justify-content: space-between;
  font-size: 13px; font-weight: 600; margin-bottom: 6px;
}
.perm-group :deep(.el-checkbox) { margin-right: 14px; }
</style>
