<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar">
        <span style="font-weight:600">选项字典</span>
        <span class="am-text-dim" style="font-size:12px">表单下拉/多选的选项在此维护；启动只补缺，不覆盖已改项。厂家类型与模型标识是父子关系。</span>
        <div class="am-flex-1" />
        <el-button type="primary" :icon="'Plus'" @click="openCreate()">新增选项</el-button>
        <el-button :icon="'Refresh'" @click="reload" />
      </div>
      <div class="dict-layout">
        <aside class="dict-groups">
          <button
            v-for="g in visibleGroups"
            :key="g.key"
            type="button"
            class="dict-group"
            :class="{ 'is-current': currentGroup === g.key }"
            @click="currentGroup = g.key"
          >
            <div>{{ g.name }}</div>
            <div class="am-text-dim" style="font-size:11px">{{ g.key }} · {{ groupCount(g) }}</div>
          </button>
        </aside>
        <section class="dict-items">
          <div v-if="!tableRows.length" class="am-empty">该分组暂无选项</div>
          <el-table
            v-else
            :data="tableRows"
            size="small"
            row-key="id"
            :tree-props="{ children: 'children' }"
            default-expand-all
          >
            <el-table-column prop="value" label="取值" min-width="180">
              <template #default="{ row }">
                <span :class="{ 'am-text-dim': row._child }">{{ row.value }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="label" label="显示名" min-width="160" />
            <el-table-column label="分组" width="110">
              <template #default="{ row }">
                <span class="am-text-dim">{{ groupName(row.group) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="sort" label="排序" width="80" />
            <el-table-column label="启用" width="80">
              <template #default="{ row }">
                <el-switch v-model="row.enabled" size="small" @change="v => toggle(row, v)" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="200">
              <template #default="{ row }">
                <el-button v-if="row.group === 'provider_kind'" link type="primary" @click="openCreate('model_slug', row)">加模型</el-button>
                <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
                <el-button link type="danger" @click="remove(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </section>
      </div>
    </div>

    <el-dialog v-model="dialog" :title="form.id ? '编辑选项' : '新增选项'" width="520">
      <el-form :model="form" label-width="88px">
        <el-form-item label="分组">
          <el-select v-model="form.group" :disabled="!!form.id" style="width:100%">
            <el-option v-for="g in dict.groups" :key="g.key" :label="`${g.name}（${g.key}）`" :value="g.key" />
          </el-select>
        </el-form-item>
        <el-form-item label="取值"><el-input v-model="form.value" :disabled="!!form.id" /></el-form-item>
        <el-form-item label="显示名"><el-input v-model="form.label" /></el-form-item>
        <el-form-item label="排序"><el-input-number v-model="form.sort" :min="0" /></el-form-item>
        <el-form-item v-if="form.group === 'provider_kind'" label="厂家 key"><el-input v-model="form.extra_key" /></el-form-item>
        <el-form-item v-if="form.group === 'provider_kind'" label="Base URL"><el-input v-model="form.extra_base_url" /></el-form-item>
        <el-form-item v-if="form.group === 'model_slug'" label="所属厂家">
          <el-select v-model="form.parent_id" style="width:100%" @change="onParentChange">
            <el-option
              v-for="p in dict.items('provider_kind', { enabledOnly: false })"
              :key="p.id"
              :label="p.label || p.value"
              :value="p.id"
            />
          </el-select>
        </el-form-item>
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
import { computed, onMounted, ref } from 'vue'
import { createDict, updateDict, deleteDict } from '@/api'
import { useDicts } from '@/composables/useDicts'
import { ElMessage, ElMessageBox } from 'element-plus'

const dict = useDicts()
const currentGroup = ref('log_level')
const dialog = ref(false)
const form = ref({})

const visibleGroups = computed(() => (dict.groups || []).filter(g => !g.parent_group))

const tableRows = computed(() => {
  const g = currentGroup.value
  const meta = (dict.groups || []).find(x => x.key === g)
  const parents = dict.items(g, { enabledOnly: false })
  if (!meta?.child_group) return parents
  const attached = new Set()
  const rows = parents.map((p) => {
    const children = dict.childrenOf(p.id).map((c) => {
      attached.add(c.id)
      return { ...c, _child: true }
    })
    return children.length ? { ...p, children } : { ...p }
  })
  for (const c of dict.items(meta.child_group, { enabledOnly: false })) {
    if (!attached.has(c.id)) rows.push({ ...c, _child: true })
  }
  return rows
})

function groupCount(g) {
  const n = (dict.grouped[g.key] || []).length
  if (!g.child_group) return n
  return n + (dict.grouped[g.child_group] || []).length
}

function groupName(key) {
  return (dict.groups || []).find(g => g.key === key)?.name || key
}

function onParentChange(id) {
  const p = dict.items('provider_kind', { enabledOnly: false }).find(x => Number(x.id) === Number(id))
  form.value.extra_kind = p?.value || ''
}

function openCreate(group, parent) {
  const g = group || currentGroup.value
  form.value = {
    group: g, value: '', label: '', sort: 10, enabled: true,
    extra_key: '', extra_base_url: '', extra_kind: parent?.value || '',
    parent_id: parent?.id || undefined
  }
  dialog.value = true
}
function openEdit(row) {
  form.value = {
    id: row.id, group: row.group, value: row.value, label: row.label, sort: row.sort, enabled: row.enabled,
    extra_key: row.extra?.key || '', extra_base_url: row.extra?.base_url || '', extra_kind: row.extra?.kind || '',
    parent_id: row.parent_id || undefined
  }
  dialog.value = true
}

function extraOf(f) {
  if (f.group === 'provider_kind') return { key: f.extra_key, base_url: f.extra_base_url }
  if (f.group === 'model_slug') return { kind: f.extra_kind }
  return undefined
}

async function submit() {
  const extra = extraOf(form.value)
  const payload = { label: form.value.label, sort: form.value.sort, enabled: form.value.enabled, extra }
  if (form.value.group === 'model_slug') payload.parent_id = form.value.parent_id || null
  if (form.value.id) {
    await updateDict(form.value.id, payload)
  } else {
    await createDict({
      group: form.value.group,
      value: form.value.value,
      label: form.value.label || form.value.value,
      sort: form.value.sort,
      extra,
      parent_id: form.value.parent_id || undefined
    })
  }
  ElMessage.success('已保存')
  dialog.value = false
  await reload()
}

async function toggle(row, v) {
  await updateDict(row.id, { enabled: v })
  await reload()
}

async function remove(row) {
  if ((row.children || []).length) {
    ElMessage.warning('请先删除该厂家下的模型标识')
    return
  }
  await ElMessageBox.confirm(`删除「${row.label || row.value}」？`, '警告', { type: 'warning' })
  await deleteDict(row.id)
  ElMessage.success('已删除')
  await reload()
}

async function reload() {
  await dict.load(true)
  if (!currentGroup.value && visibleGroups.value.length) currentGroup.value = visibleGroups.value[0].key
}

onMounted(async () => {
  await dict.load()
  if (visibleGroups.value.length) currentGroup.value = visibleGroups.value[0].key
})
</script>

<style scoped>
.dict-layout { display: grid; grid-template-columns: 220px 1fr; gap: 16px; margin-top: 12px; }
.dict-groups { display: flex; flex-direction: column; gap: 4px; }
.dict-group {
  text-align: left; border: 1px solid var(--am-border); background: var(--am-bg-inset);
  color: var(--am-text); border-radius: 10px; padding: 10px 12px; cursor: pointer;
}
.dict-group:hover { border-color: var(--am-border-strong); }
.dict-group.is-current { border-color: var(--am-primary); background: var(--am-primary-soft); }
@media (max-width: 768px) {
  .dict-layout { grid-template-columns: 1fr; }
}
</style>
