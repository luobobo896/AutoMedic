<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar">
        <el-button type="primary" :icon="'Plus'" @click="openCreate">创建令牌</el-button>
        <div class="am-flex-1" />
        <el-button :icon="'Refresh'" aria-label="刷新" @click="load" />
      </div>
      <div class="am-field-help" style="margin:-4px 0 12px">
        采集器用请求头 <span class="am-mono">X-AM-Token</span> 投递事件。明文只显示一次。
      </div>

      <el-table :data="list" v-loading="loading" size="small" style="width:100%">
        <template #empty>
          <div class="am-empty">
            <div class="am-empty__title">还没有投递令牌</div>
            <div class="am-empty__desc">告警源带着令牌 POST 进来，平台才知道事件属于哪个项目。</div>
            <div class="am-empty__actions">
              <el-button type="primary" :icon="'Plus'" @click="openCreate">创建第一个令牌</el-button>
            </div>
          </div>
        </template>
        <el-table-column prop="name" label="名称" min-width="160" />
        <el-table-column label="项目" min-width="140">
          <template #default="{ row }">{{ row.project?.name || '-' }}</template>
        </el-table-column>
        <el-table-column prop="prefix" label="前缀" min-width="140">
          <template #default="{ row }"><span class="am-mono">{{ row.prefix || '-' }}</span></template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '禁用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="IP 白名单" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ row.allow_cidr || '不限制' }}</template>
        </el-table-column>
        <el-table-column label="有效期" width="170">
          <template #default="{ row }">{{ row.expires_at ? formatTime(row.expires_at) : '永久' }}</template>
        </el-table-column>
        <el-table-column label="最近使用" width="170">
          <template #default="{ row }">
            <span :class="{ 'am-text-dim': !row.last_used_at }">{{ row.last_used_at ? formatTime(row.last_used_at) : '从未使用' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="showCurl(row)">接入示例</el-button>
            <el-button link type="primary" @click="toggle(row)">{{ row.enabled ? '禁用' : '启用' }}</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination class="am-pagination"
        layout="total, sizes, prev, pager, next" :total="total"
        v-model:current-page="query.page" v-model:page-size="query.page_size"
        @current-change="load" @size-change="search" />
    </div>

    <el-dialog v-model="dialog" title="创建项目令牌" class="am-dialog-form" width="520">
      <el-form :model="form" label-width="110px">
        <el-form-item label="所属项目" required>
          <el-select v-model="form.project_id" style="width:100%">
            <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="如：loki-collector" />
        </el-form-item>
        <el-form-item label="IP 白名单">
          <el-input v-model="form.allow_cidr" placeholder="逗号分隔 CIDR，留空不限制" />
          <div class="am-field-help">只允许这些网段投递；留空则不限制来源 IP。</div>
        </el-form-item>
        <el-form-item label="有效期">
          <el-date-picker v-model="form.expires_at" type="datetime" placeholder="留空为永久" style="width:100%" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="curlDialog" title="接入示例" width="700">
      <div class="am-diff" style="max-height:380px">{{ curlText }}</div>
      <template #footer>
        <el-button @click="curlDialog = false">关闭</el-button>
        <el-button type="primary" @click="copyCurl">复制</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { listTokens, createToken, updateToken, deleteToken, listProjects } from '@/api'
import { formatTime, copyText } from '@/utils/format'
import { ElMessage, ElMessageBox } from 'element-plus'

const list = ref([])
const projects = ref([])
const total = ref(0)
const loading = ref(false)
const saving = ref(false)
const dialog = ref(false)
const curlDialog = ref(false)
const curlText = ref('')
const plainTokens = ref({})
const form = ref({})
const query = reactive({ page: 1, page_size: 20 })

function openCreate() {
  form.value = { project_id: null, name: '', allow_cidr: '', expires_at: null }
  dialog.value = true
}

async function load() {
  loading.value = true
  try {
    const r = await listTokens(query)
    list.value = r.data?.list || []
    total.value = r.data?.total || 0
  } finally { loading.value = false }
}

// 每页条数变化后必须回到第 1 页，否则停留在旧页码可能拿到空列表
function search() {
  query.page = 1
  return load()
}

async function submit() {
  if (saving.value) return
  if (!form.value.project_id) {
    ElMessage.error('请选择所属项目')
    return
  }
  if (!form.value.name) {
    ElMessage.error('请填写名称')
    return
  }
  saving.value = true
  try {
    const payload = {
      project_id: form.value.project_id,
      name: form.value.name,
      allow_cidr: form.value.allow_cidr || ''
    }
    if (form.value.expires_at) payload.expires_at = new Date(form.value.expires_at).toISOString()
    const r = await createToken(payload)
    plainTokens.value[r.data.token.id] = r.data.plain_token
    dialog.value = false
    await ElMessageBox.alert(
      `令牌仅显示一次，请立即保存：\n\n${r.data.plain_token}`,
      '创建成功', { confirmButtonText: '我已保存' }
    ).catch(() => { /* ESC / 点关闭同样按已读处理 */ })
    await load()
  } finally {
    saving.value = false
  }
}

async function toggle(row) {
  await updateToken(row.id, { enabled: !row.enabled })
  ElMessage.success('已更新')
  load()
}

async function remove(row) {
  const ok = await ElMessageBox.confirm(`确认删除令牌「${row.name}」？删除后使用该令牌的采集器将立即失效`, '警告', { type: 'warning' }).catch(() => false)
  if (!ok) return
  await deleteToken(row.id)
  ElMessage.success('已删除')
  load()
}

function showCurl(row) {
  const t = plainTokens.value[row.id] || '<令牌明文，仅在创建时可见>'
  curlText.value = `# 单条投递
curl -X POST http://<automedic-host>/api/v1/ingest/events \\
  -H "Content-Type: application/json" \\
  -H "X-AM-Token: ${t}" \\
  -d '{
    "source": "sentry",
    "level": "fatal",
    "title": "panic: nil pointer dereference in order service",
    "message": "order service panic",
    "stack": "panic: runtime error: invalid memory address\\n at internal/order/service.go:128",
    "fingerprint": "order-nil-ctx-001",
    "payload": { "env": "prod", "service": "order" }
  }'

# 批量投递
curl -X POST http://<automedic-host>/api/v1/ingest/events/batch \\
  -H "Content-Type: application/json" \\
  -H "X-AM-Token: ${t}" \\
  -d '{ "events": [ { ... }, { ... } ] }'`
  curlDialog.value = true
}

function copyCurl() { copyText(curlText.value) }

onMounted(async () => {
  const p = await listProjects({ page_size: 100 })
  projects.value = p.data?.list || []
  form.value = { project_id: projects.value[0]?.id, name: 'collector' }
  load()
})
</script>
