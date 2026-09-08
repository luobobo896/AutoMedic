<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar">
        <span style="font-weight:600">选项字典</span>
        <span class="am-text-dim" style="font-size:12px">表单下拉/多选的选项在此维护；启动只补缺，不覆盖已改项</span>
        <div class="am-flex-1" />
        <el-button type="primary" :icon="'Plus'" @click="openCreate">新增选项</el-button>
        <el-button :icon="'Refresh'" @click="reload" />
      </div>
      <div class="dict-layout">
        <aside class="dict-groups">
          <button
            v-for="g in dict.groups"
            :key="g.key"
            type="button"
            class="dict-group"
            :class="{ 'is-current': currentGroup === g.key }"
            @click="currentGroup = g.key"
          >
            <div>{{ g.name }}</div>
            <div class="am-text-dim" style="font-size:11px">{{ g.key }} · {{ (dict.grouped[g.key] || []).length }}</div>
          </button>
        </aside>
        <section class="dict-items">
          <el-table :data="rows" size="small">
            <el-table-column prop="value" label="取值" min-width="140" />
            <el-table-column prop="label" label="显示名" min-width="140" />
            <el-table-column prop="sort" label="排序" width="80" />
            <el-table-column label="启用" width="80">
              <template #default="{ row }">
                <el-switch v-model="row.enabled" size="small" @change="v => toggle(row, v)" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="140">
              <template #default="{ row }">
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
        <el-form-item v-if="form.group === 'model_slug'" label="厂家类型">
          <el-select v-model="form.extra_kind" style="width:100%">
            <el-option v-for="p in dict.providerPresets()" :key="p.kind" :label="p.name" :value="p.kind" />
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

const rows = computed(() => dict.items(currentGroup.value, { enabledOnly: false }))

function openCreate() {
  form.value = { group: currentGroup.value, value: '', label: '', sort: 10, enabled: true, extra_key: '', extra_base_url: '', extra_kind: '' }
  dialog.value = true
}
function openEdit(row) {
  form.value = {
    id: row.id, group: row.group, value: row.value, label: row.label, sort: row.sort, enabled: row.enabled,
    extra_key: row.extra?.key || '', extra_base_url: row.extra?.base_url || '', extra_kind: row.extra?.kind || ''
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
  if (form.value.id) {
    await updateDict(form.value.id, { label: form.value.label, sort: form.value.sort, enabled: form.value.enabled, extra })
  } else {
    await createDict({ group: form.value.group, value: form.value.value, label: form.value.label || form.value.value, sort: form.value.sort, extra })
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
  await ElMessageBox.confirm(`删除「${row.label || row.value}」？`, '警告', { type: 'warning' })
  await deleteDict(row.id)
  ElMessage.success('已删除')
  await reload()
}

async function reload() {
  await dict.load(true)
  if (!currentGroup.value && dict.groups.length) currentGroup.value = dict.groups[0].key
}

onMounted(async () => {
  await dict.load()
  if (dict.groups.length) currentGroup.value = dict.groups[0].key
})
</script>

<style scoped>
.dict-layout { display: grid; grid-template-columns: 220px 1fr; gap: 16px; margin-top: 12px; }
.dict-groups { display: flex; flex-direction: column; gap: 4px; }
.dict-group {
  text-align: left; border: 1px solid var(--am-border); background: var(--am-bg-inset);
  color: var(--am-text); border-radius: 10px; padding: 10px 12px; cursor: pointer;
}
.dict-group.is-current { border-color: var(--am-primary); background: var(--am-primary-soft); }
@media (max-width: 768px) {
  .dict-layout { grid-template-columns: 1fr; }
}
</style>
