<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar">
        <span class="am-card__title">修复规则</span>
        <span class="am-text-dim" style="font-size: var(--am-font-xs)">规则决定告警是否进入代码修复流程，未命中规则的事件默认忽略</span>
        <div class="am-flex-1" />
        <el-button type="primary" :icon="'Plus'" @click="openCreate">新增规则</el-button>
        <el-button :icon="'Refresh'" @click="load" />
      </div>

      <el-table :data="list" v-loading="loading" size="small">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="规则名称" width="180" />
        <el-table-column label="项目" width="140">
          <template #default="{ row }">{{ row.project?.name || '-' }}</template>
        </el-table-column>
        <el-table-column prop="priority" label="优先级" width="80" />
        <el-table-column label="级别" width="110">
          <template #default="{ row }">{{ row.levels || '不限' }}</template>
        </el-table-column>
        <el-table-column label="命中关键字" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">{{ row.keywords || '-' }}</template>
        </el-table-column>
        <el-table-column label="排除关键字" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <el-tag v-for="k in splitTags(row.exclude_keywords)" :key="k" size="small" type="info" style="margin-right:4px">{{ k }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="阈值/冷却" width="150">
          <template #default="{ row }">
            {{ row.min_count }}次/{{ row.window_sec }}s · 冷却{{ row.cooldown_sec }}s
          </template>
        </el-table-column>
        <el-table-column label="动作" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.action === 'fix' ? 'success' : 'info'">{{ row.action === 'fix' ? '修复' : '忽略' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" size="small" @change="v => toggle(row, v)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="dialog" :title="form.id ? '编辑规则' : '新增规则'" width="720">
      <el-form :model="form" label-width="120px">
        <el-form-item label="所属项目">
          <el-select v-model="form.project_id" style="width:100%">
            <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="规则名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="优先级"><el-input-number v-model="form.priority" :min="0" :max="999" /></el-form-item>
        <el-form-item label="日志级别">
          <el-select v-model="form.levels" multiple clearable collapse-tags style="width:100%" placeholder="留空不限">
            <el-option v-for="o in dict.options('log_level')" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="命中关键字">
          <el-select v-model="form.keywords" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%" placeholder="勾选或输入后回车">
            <el-option v-for="k in dict.values('hit_keyword')" :key="k" :label="k" :value="k" />
          </el-select>
        </el-form-item>
        <el-form-item label="必须全命中">
          <el-select v-model="form.all_keywords" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%" placeholder="可选">
            <el-option v-for="k in dict.values('hit_keyword')" :key="'all-'+k" :label="k" :value="k" />
          </el-select>
        </el-form-item>
        <el-form-item label="排除关键字">
          <el-select v-model="form.exclude_keywords" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%" placeholder="业务拒绝 / 第三方故障等">
            <el-option v-for="k in dict.values('exclude_keyword')" :key="k" :label="k" :value="k" />
          </el-select>
        </el-form-item>
        <el-form-item label="来源白名单">
          <el-select v-model="form.sources" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%" placeholder="留空不限">
            <el-option v-for="s in dict.values('event_source')" :key="s" :label="s" :value="s" />
          </el-select>
        </el-form-item>
        <el-form-item label="排除来源">
          <el-select v-model="form.exclude_sources" multiple filterable allow-create default-first-option collapse-tags
            style="width:100%" placeholder="如 biz-reject">
            <el-option v-for="s in dict.values('event_source')" :key="'ex-'+s" :label="s" :value="s" />
          </el-select>
        </el-form-item>
        <el-form-item label="正则"><el-input v-model="form.pattern" placeholder="可选，命中才触发" /></el-form-item>
        <el-form-item label="频次阈值">
          <el-input-number v-model="form.min_count" :min="1" /> 次 /
          <el-select v-model="form.window_sec" style="width:140px">
            <el-option v-for="o in dict.numberOptions('window_sec')" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="冷却时间">
          <el-select v-model="form.cooldown_sec" style="width:160px">
            <el-option v-for="o in dict.numberOptions('cooldown_sec')" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="动作">
          <el-radio-group v-model="form.action">
            <el-radio value="fix">触发修复</el-radio>
            <el-radio value="ignore">忽略</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="限定仓库">
          <el-select v-model="repoIds" multiple clearable style="width:100%" placeholder="留空则使用项目下所有启用仓库">
            <el-option v-for="r in repos" :key="r.id" :label="r.name" :value="r.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="指定模型">
          <el-select v-model="form.model_id" clearable style="width:100%" placeholder="留空继承项目">
            <el-option v-for="m in models" :key="m.id" :label="`${m.provider?.name} / ${m.name}`" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="修复模式">
          <el-radio-group v-model="form.fix_mode">
            <el-radio value="">继承项目</el-radio>
            <el-radio value="auto">全自动</el-radio>
            <el-radio value="semi">半自动</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="附加要求">
          <el-input v-model="form.prompt_template" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item label="最大重试"><el-input-number v-model="form.max_retries" :min="0" :max="10" /></el-form-item>
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
import { ref, onMounted } from 'vue'
import { listRules, createRule, updateRule, deleteRule, listProjects, listRepos, listModels } from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useDicts, splitCSV, joinCSV } from '@/composables/useDicts'

const dict = useDicts()
const list = ref([])
const projects = ref([])
const repos = ref([])
const models = ref([])
const loading = ref(false)
const dialog = ref(false)
const form = ref({})
const repoIds = ref([])

const defaultForm = () => ({
  enabled: true, action: 'fix', priority: 0, min_count: 1, window_sec: 300,
  cooldown_sec: 600, max_retries: 2, fix_mode: '',
  levels: ['fatal', 'error'], keywords: [], all_keywords: [],
  exclude_keywords: [], sources: [], exclude_sources: []
})

async function load() {
  loading.value = true
  try {
    const r = await listRules({ page_size: 200 })
    list.value = r.data?.list || []
  } finally { loading.value = false }
}

function splitTags(s) {
  return (s || '').split(/[,，]/).map(x => x.trim()).filter(Boolean).slice(0, 4)
}

function openCreate() {
  form.value = { ...defaultForm(), project_id: projects.value[0]?.id }
  repoIds.value = []
  dialog.value = true
}

function openEdit(row) {
  form.value = {
    id: row.id, project_id: row.project_id, name: row.name, enabled: row.enabled, priority: row.priority,
    levels: splitCSV(row.levels), sources: splitCSV(row.sources), keywords: splitCSV(row.keywords),
    all_keywords: splitCSV(row.all_keywords), exclude_keywords: splitCSV(row.exclude_keywords),
    exclude_sources: splitCSV(row.exclude_sources), pattern: row.pattern,
    min_count: row.min_count, window_sec: row.window_sec, cooldown_sec: row.cooldown_sec,
    action: row.action, model_id: row.model_id, fix_mode: row.fix_mode || '',
    max_retries: row.max_retries, prompt_template: row.prompt_template, description: row.description
  }
  repoIds.value = (row.repo_ids || '').split(',').filter(Boolean).map(Number)
  dialog.value = true
}

function rulePayload(f, ids) {
  return {
    project_id: f.project_id, name: f.name, enabled: f.enabled !== false, priority: f.priority || 0,
    levels: joinCSV(f.levels), sources: joinCSV(f.sources), keywords: joinCSV(f.keywords),
    all_keywords: joinCSV(f.all_keywords), exclude_keywords: joinCSV(f.exclude_keywords),
    exclude_sources: joinCSV(f.exclude_sources), pattern: f.pattern || '',
    min_count: f.min_count, window_sec: f.window_sec, cooldown_sec: f.cooldown_sec,
    action: f.action || 'fix', repo_ids: (ids || []).join(','),
    model_id: f.model_id || null, fix_mode: f.fix_mode || '',
    max_retries: f.max_retries, prompt_template: f.prompt_template || '', description: f.description || ''
  }
}

async function submit() {
  const payload = rulePayload(form.value, repoIds.value)
  if (form.value.id) await updateRule(form.value.id, payload)
  else await createRule(payload)
  ElMessage.success('已保存')
  dialog.value = false
  load()
}

async function toggle(row, v) {
  await updateRule(row.id, { enabled: v })
  ElMessage.success('已更新')
}

async function remove(row) {
  const ok = await ElMessageBox.confirm(`确认删除规则「${row.name}」？`, '警告', { type: 'warning' }).catch(() => false)
  if (!ok) return
  await deleteRule(row.id)
  ElMessage.success('已删除')
  load()
}

onMounted(async () => {
  const [p, r, m] = await Promise.all([listProjects({ page_size: 100 }), listRepos({ page_size: 200 }), listModels(), dict.load()])
  projects.value = p.data?.list || []
  repos.value = r.data?.list || []
  models.value = m.data || []
  load()
})
</script>
