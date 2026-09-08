<template>
  <div class="am-page">
    <div class="am-page-head">
      <div>
        <h2 class="am-page-title">角色与权限</h2>
        <p class="am-page-desc">RBAC：用户绑定角色，角色勾选资源权限。内置角色可改权限、不可删除。</p>
      </div>
      <el-button class="am-btn-soft" :icon="'Plus'" @click="openCreate">新增角色</el-button>
    </div>

    <div class="role-layout">
      <aside class="role-list" aria-label="角色列表">
        <button
          v-for="r in roles"
          :key="r.id"
          type="button"
          class="role-item"
          :class="{ 'is-current': current?.id === r.id }"
          @click="select(r)"
        >
          <div class="role-item-main">
            <div class="role-name">{{ r.name }}</div>
            <div class="role-code am-mono">{{ r.code }}</div>
          </div>
          <span v-if="r.builtin" class="am-pill">内置</span>
        </button>
        <div v-if="!roles.length" class="am-empty">暂无角色</div>
      </aside>

      <section class="role-editor" v-if="current">
        <div class="role-editor-head">
          <div>
            <div class="role-editor-title">{{ form.name || current.name }}</div>
            <div class="am-text-dim" style="font-size:12px;margin-top:4px">
              <template v-if="isSuperLocked">平台超管始终拥有全部权限，不在此树中裁剪。</template>
              <template v-else-if="current.builtin">内置角色：可调整权限树，不可删除编码。</template>
              <template v-else>自定义角色：勾选资源下的动作后保存。</template>
            </div>
          </div>
          <div class="role-editor-actions">
            <el-button
              v-if="current.builtin && !isSuperLocked"
              class="am-btn-soft"
              @click="restoreDefault"
            >恢复默认</el-button>
            <el-button
              class="am-icon-btn am-icon-danger"
              link
              :disabled="current.builtin"
              @click="remove(current)"
            >
              <el-icon><Delete /></el-icon>
            </el-button>
            <el-button type="primary" class="am-btn-primary" :loading="saving" @click="submit">保存</el-button>
          </div>
        </div>

        <el-form :model="form" label-position="top" class="am-form">
          <div class="am-form-grid">
            <el-form-item label="角色名称">
              <el-input v-model="form.name" :disabled="isSuperLocked" />
            </el-form-item>
            <el-form-item label="角色编码">
              <el-input v-model="form.code" disabled />
            </el-form-item>
          </div>
          <el-form-item label="备注">
            <el-input v-model="form.remark" :disabled="isSuperLocked" />
          </el-form-item>
          <el-form-item label="权限">
            <el-tree
              ref="treeRef"
              class="perm-tree"
              :data="editorTreeData"
              node-key="id"
              show-checkbox
              default-expand-all
              :check-strictly="false"
              :props="{ label: 'label', children: 'children', disabled: 'disabled' }"
            />
          </el-form-item>
        </el-form>
      </section>

      <div v-else class="role-editor am-empty">选择左侧角色以编辑权限树</div>
    </div>

    <el-dialog v-model="createDialog" title="新增角色" width="720" class="am-dialog" @opened="onCreateOpened">
      <p class="am-dialog-desc">先填名称与编码，再按资源勾选动作。保存后出现在左侧列表。</p>
      <el-form :model="createForm" label-position="top" class="am-form">
        <div class="am-form-grid">
          <el-form-item label="角色名称"><el-input v-model="createForm.name" /></el-form-item>
          <el-form-item label="角色编码"><el-input v-model="createForm.code" placeholder="如 ops" /></el-form-item>
        </div>
        <el-form-item label="备注"><el-input v-model="createForm.remark" /></el-form-item>
        <el-form-item label="权限">
          <el-tree
            ref="createTreeRef"
            class="perm-tree"
            :data="treeData"
            node-key="id"
            show-checkbox
            default-expand-all
            :check-strictly="false"
            :props="{ label: 'label', children: 'children' }"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button class="am-btn-soft" @click="createDialog = false">取消</el-button>
        <el-button type="primary" class="am-btn-primary" :loading="saving" @click="submitCreate">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete } from '@element-plus/icons-vue'
import { listRoles, createRole, updateRole, deleteRole, listPermissions } from '@/api'

const roles = ref([])
const catalog = ref([])
const current = ref(null)
const form = ref({ permissions: [] })
const treeRef = ref(null)
const createTreeRef = ref(null)
const createDialog = ref(false)
const createForm = ref({ name: '', code: '', remark: '', permissions: [] })
const saving = ref(false)

const isSuperLocked = computed(() => current.value?.code === 'super_admin' && current.value?.tenant_id === 0)

function buildTree(lockLeaves) {
  const groups = []
  const index = {}
  for (const p of catalog.value) {
    const g = p.group || '其他'
    if (!index[g]) {
      index[g] = { id: 'g:' + g, label: g, children: [], disabled: lockLeaves }
      groups.push(index[g])
    }
    index[g].children.push({
      id: p.code,
      label: p.name,
      code: p.code,
      disabled: lockLeaves
    })
  }
  return groups
}

const treeData = computed(() => buildTree(false))
const editorTreeData = computed(() => buildTree(isSuperLocked.value))

function leafKeys(codes) {
  return (codes || []).filter((c) => typeof c === 'string' && !c.startsWith('g:'))
}

function checkedFromTree(tree) {
  if (!tree) return []
  const nodes = tree.getCheckedNodes(true, false)
  return nodes.filter((n) => n.code).map((n) => n.code)
}

function applyChecked(tree, codes) {
  if (!tree) return
  tree.setCheckedKeys(leafKeys(codes))
}

async function load(selectId) {
  const [r, c] = await Promise.all([listRoles(), listPermissions()])
  roles.value = r.data || []
  catalog.value = c.data || []
  const keep = selectId || current.value?.id
  const next = roles.value.find((x) => x.id === keep) || roles.value[0] || null
  if (next) select(next)
  else current.value = null
}

function select(r) {
  current.value = r
  form.value = {
    id: r.id,
    name: r.name,
    code: r.code,
    remark: r.remark || '',
    permissions: [...(r.permissions || [])]
  }
  nextTick(() => applyChecked(treeRef.value, form.value.permissions))
}

function openCreate() {
  createForm.value = { name: '', code: '', remark: '', permissions: [] }
  createDialog.value = true
}

function onCreateOpened() {
  nextTick(() => applyChecked(createTreeRef.value, []))
}

function restoreDefault() {
  const def = current.value?.default_permissions || []
  form.value.permissions = [...def]
  applyChecked(treeRef.value, def)
}

async function submit() {
  if (isSuperLocked.value) {
    ElMessage.info('平台超管权限不可裁剪')
    return
  }
  saving.value = true
  try {
    const permissions = checkedFromTree(treeRef.value)
    await updateRole(form.value.id, {
      name: form.value.name,
      remark: form.value.remark,
      permissions
    })
    ElMessage.success('已保存')
    await load(form.value.id)
  } finally {
    saving.value = false
  }
}

async function submitCreate() {
  if (!createForm.value.name || !createForm.value.code) {
    ElMessage.warning('请填写角色名称与编码')
    return
  }
  saving.value = true
  try {
    const permissions = checkedFromTree(createTreeRef.value)
    const created = await createRole({
      name: createForm.value.name,
      code: createForm.value.code,
      remark: createForm.value.remark,
      permissions
    })
    ElMessage.success('已创建')
    createDialog.value = false
    await load(created.data?.id)
  } finally {
    saving.value = false
  }
}

async function remove(r) {
  await ElMessageBox.confirm(`确认删除角色「${r.name}」？`, '警告', { type: 'warning' })
  await deleteRole(r.id)
  ElMessage.success('已删除')
  current.value = null
  await load()
}

onMounted(load)
</script>

<style scoped>
.role-layout {
  display: grid;
  grid-template-columns: minmax(220px, 280px) 1fr;
  gap: 14px;
  align-items: start;
}
.role-list {
  background: var(--am-bg-elevated);
  border: 1px solid var(--am-border);
  border-radius: var(--am-radius);
  padding: 8px;
  min-height: 420px;
}
.role-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  text-align: left;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 10px;
  padding: 10px 12px;
  color: var(--am-text);
  cursor: pointer;
}
.role-item:hover { background: var(--am-bg-inset); }
.role-item.is-current {
  background: var(--am-primary-soft);
  border-color: var(--am-border-strong);
}
.role-item-main { flex: 1; min-width: 0; }
.role-name { font-size: 14px; font-weight: 600; }
.role-code { font-size: 12px; color: var(--am-text-dim); margin-top: 2px; }
.role-editor {
  background: var(--am-bg-elevated);
  border: 1px solid var(--am-border);
  border-radius: var(--am-radius);
  padding: 18px 20px;
  min-height: 420px;
}
.role-editor-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}
.role-editor-title { font-size: 16px; font-weight: 600; }
.role-editor-actions { display: flex; align-items: center; gap: 8px; }
.perm-tree {
  background: var(--am-bg-inset);
  border: 1px solid var(--am-border);
  border-radius: 12px;
  padding: 10px 12px;
  max-height: 56vh;
  overflow: auto;
}
.perm-tree :deep(.el-tree-node__content) {
  height: 32px;
  border-radius: 8px;
}
.perm-tree :deep(.el-tree-node__content:hover) {
  background: var(--am-bg-elevated);
}
@media (max-width: 768px) {
  .role-layout { grid-template-columns: 1fr; }
}
</style>
