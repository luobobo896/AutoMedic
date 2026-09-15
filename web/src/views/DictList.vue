<template>
  <div class="am-page">
    <div class="am-page-head">
      <div>
        <div class="am-page-head__crumb">首页</div>
        <h1 class="am-page-head__title">选项字典</h1>
        <p class="am-page-head__desc">
          表单枚举的唯一来源。厂家类型、模型标识、温度等下拉都引用这里；启动只补缺，不覆盖已改项。
        </p>
      </div>
      <div class="am-page-head__actions">
        <el-button :icon="'Refresh'" aria-label="刷新" @click="reload" />
        <el-button type="primary" :icon="'Plus'" @click="openCreate()">新增选项</el-button>
      </div>
    </div>

    <div class="am-panel">
      <div class="am-panel__body">
      <div class="dict-layout">
        <aside class="dict-groups">
          <button
            v-for="g in visibleGroups"
            :key="g.key"
            type="button"
            class="dict-group am-subnav__item"
            :class="{ 'is-current': currentGroup === g.key }"
            @click="currentGroup = g.key"
          >
            <span class="am-subnav__name">{{ g.name }}</span>
            <span class="am-subnav__n">{{ groupCount(g) }}</span>
          </button>
        </aside>
        <section class="dict-items">
          <div class="dict-head">
            <div>
              <h2 class="am-panel__title">{{ groupName(currentGroup) }}</h2>
              <p class="am-panel__desc">{{ groupDesc(currentGroup) }}</p>
            </div>
            <div class="dict-head__refs">
              <span>被 <b>{{ groupUsage.providers }}</b> 个厂家引用</span>
              <span>被 <b>{{ groupUsage.models }}</b> 个模型引用</span>
            </div>
          </div>
          <div v-if="!tableRows.length" class="am-empty">
            <div class="am-empty__title">该分组暂无选项</div>
            <div class="am-empty__desc">新增一个选项后，模型配置、规则等页面即可在下拉里选到它。</div>
            <div class="am-empty__actions">
              <el-button size="small" type="primary" @click="openCreate()">新增选项</el-button>
            </div>
          </div>
          <el-table
            v-else
            :data="tableRows"
            size="small"
            row-key="id"
            :tree-props="{ children: 'children' }"
            default-expand-all
          >
            <el-table-column prop="value" label="取值" min-width="160">
              <template #default="{ row }">
                <span :class="{ 'am-text-dim': row._child }">{{ row.value }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="label" label="显示名" min-width="140" />
            <el-table-column label="分组" width="96">
              <template #default="{ row }">
                <span class="am-text-dim">{{ groupName(row.group) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="sort" label="排序" width="72" />
            <el-table-column label="引用" width="118">
              <template #default="{ row }">
                <el-popover v-if="usageTotal(row)" placement="left" width="260" trigger="hover">
                  <template #reference>
                    <span class="am-ref am-ref--on">
                      <span v-if="row.usage.providers">厂家 {{ row.usage.providers }}</span>
                      <span v-if="row.usage.models">模型 {{ row.usage.models }}</span>
                    </span>
                  </template>
                  <div class="am-field__label">引用方</div>
                  <ul class="am-ref__list">
                    <li v-for="s in row.usage.samples" :key="s">{{ s }}</li>
                  </ul>
                  <router-link class="am-link" :to="usageLink(row)">在模型配置中查看</router-link>
                </el-popover>
                <span v-else class="am-text-faint am-hint">未引用</span>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="72">
              <template #default="{ row }">
                <el-switch v-model="row.enabled" size="small" :disabled="saving" @change="v => toggle(row, v)" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="190">
              <template #default="{ row }">
                <el-button v-if="row.group === 'provider_kind'" link type="primary" @click="openCreate('model_slug', row)">加模型标识</el-button>
                <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
                <el-button link type="danger" @click="remove(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </section>
      </div>
      </div>
    </div>

    <el-dialog v-model="dialog" :title="form.id ? '编辑选项' : '新增选项'" width="520">
      <el-form :model="form" label-width="88px">
            <el-form-item label="分组">
              <el-select v-model="form.group" :disabled="!!form.id" style="width:100%">
                <el-option v-for="g in dict.groups" :key="g.key" :label="`${g.name}（${g.key}）`" :value="g.key" />
              </el-select>
        </el-form-item>
        <el-form-item label="取值">
          <el-input v-model="form.value" :disabled="!!form.id" placeholder="保存后不可修改，例如 deepseek" />
          <div class="am-field__help">程序里比较用的值；保存后不可改，改动会让引用它的配置失配。</div>
        </el-form-item>
        <el-form-item label="显示名">
          <el-input v-model="form.label" placeholder="下拉里展示的文字" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort" :min="0" />
          <div class="am-field__help">数字越小越靠前。</div>
        </el-form-item>
        <el-form-item v-if="form.group === 'provider_kind'" label="厂家 key">
          <el-input v-model="form.extra_key" />
          <div class="am-field__help">选中该类型时自动填进「新增厂家」的标识 key。</div>
        </el-form-item>
        <el-form-item v-if="form.group === 'provider_kind'" label="Base URL">
          <el-input v-model="form.extra_base_url" />
          <div class="am-field__help">选中该类型时自动带出的 OpenAI 兼容端点地址。</div>
        </el-form-item>
        <el-form-item v-if="form.group === 'model_slug'" label="所属厂家">
          <el-select v-model="form.parent_id" style="width:100%" @change="onParentChange">
            <el-option
              v-for="p in dict.items('provider_kind', { enabledOnly: false })"
              :key="p.id"
              :label="p.label || p.value"
              :value="p.id"
            />
          </el-select>
          <div class="am-field__help">模型标识挂在厂家类型下，模型配置里按厂家过滤可选项。</div>
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { createDict, updateDict, deleteDict } from '@/api'
import { useDicts } from '@/composables/useDicts'
import { ElMessage, ElMessageBox } from 'element-plus'

const dict = useDicts()
const route = useRoute()
const currentGroup = ref('log_level')
const dialog = ref(false)
const saving = ref(false)
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

// 每个分组的用途说明：字典项被谁消费，写清楚才不会乱加/乱删
const GROUP_DESC = {
  log_level: '告警级别选项，事件入站与规则条件都从这里取值。',
  hit_keyword: '规则「命中关键字」的候选项，命中任一即触发。',
  exclude_keyword: '规则「排除关键字」的候选项，用于拦下第三方故障与业务拒绝。',
  event_source: '事件来源标识（sentry / loki / k8s 等），规则可按来源过滤。',
  language: '仓库语言选项，用于 dsh 定位代码与生成上下文。',
  git_branch: '新建仓库时的默认分支候选。',
  code_path: '仓库关注路径候选，缩小 dsh 的代码检索范围。',
  window_sec: '规则频次窗口（秒），窗口内达到次数才触发。',
  cooldown_sec: '同一指纹的冷却时间（秒），避免抖动重复修复。',
  temperature: '模型温度候选值，模型配置的高级设置里引用。',
  provider_kind: '厂家类型，模型配置「新增厂家」时从这里选择。',
  model_slug: '模型标识，挂在对应厂家类型下，模型配置里按厂家过滤。'
}
const groupDesc = key => GROUP_DESC[key] || ''

const usageTotal = row => (row.usage?.providers || 0) + (row.usage?.models || 0)

// 分组级引用汇总：一眼看出这个分组有没有被业务真正消费
const groupUsage = computed(() => {
  let providers = 0
  let models = 0
  for (const row of tableRows.value) {
    providers += row.usage?.providers || 0
    models += row.usage?.models || 0
  }
  return { providers, models }
})

function usageLink(row) {
  if (row.group === 'model_slug') return { path: '/models', query: { slug: row.value } }
  return { path: '/models', query: { kind: row.value } }
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
  if (saving.value) return
  saving.value = true
  try {
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
  } finally { saving.value = false }
}

async function toggle(row, v) {
  if (saving.value) return
  saving.value = true
  try {
    await updateDict(row.id, { enabled: v })
    await reload()
  } finally { saving.value = false }
}

async function remove(row) {
  if (saving.value) return
  saving.value = true
  try {
    if ((row.children || []).length) {
      ElMessage.warning('请先删除该厂家下的模型标识')
      return
    }
    const ok = await ElMessageBox.confirm(`删除「${row.label || row.value}」？`, '警告', { type: 'warning' }).catch(() => false)
    if (!ok) return
    await deleteDict(row.id)
    ElMessage.success('已删除')
    await reload()
  } finally { saving.value = false }
}

async function reload() {
  await dict.load(true)
  if (!currentGroup.value && visibleGroups.value.length) currentGroup.value = visibleGroups.value[0].key
}

onMounted(async () => {
  await dict.load()
  applyRouteGroup()
})

// 从模型配置等页面带 ?group= 跳进来时定位分组；
// 同页再次点击链接只改 query 不会重新挂载组件，所以必须 watch
watch(() => route.query.group, applyRouteGroup)

function applyRouteGroup() {
  const want = String(route.query.group || '')
  const hit = visibleGroups.value.find(g => g.key === want)
  if (hit) currentGroup.value = hit.key
  else if (!currentGroup.value && visibleGroups.value.length) currentGroup.value = visibleGroups.value[0].key
}
</script>

<style scoped>
.dict-layout { display: grid; grid-template-columns: 220px minmax(0, 1fr); gap: 16px; margin-top: 12px; }
.dict-groups { display: flex; flex-direction: column; gap: 4px; }
.dict-items { min-width: 0; }
.dict-group {
  text-align: left; border: 1px solid transparent; background: transparent;
  color: var(--am-text); border-radius: var(--am-radius-sm); padding: 9px 12px; cursor: pointer;
}
.dict-group:hover { background: var(--am-bg-inset); }
.dict-group.is-current { background: var(--am-primary-soft); border-color: transparent; }
.dict-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; flex-wrap: wrap; margin-bottom: 12px; }
.dict-head__refs { display: flex; align-items: center; gap: 16px; font-size: var(--am-font-sm); color: var(--am-text-dim); }
.dict-head__refs b { color: var(--am-text); font-weight: 600; }
@media (max-width: 767.98px) {
  .dict-layout { grid-template-columns: 1fr; }
}
</style>
