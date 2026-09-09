<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar">
        <span style="font-weight:600">项目令牌</span>
        <span class="am-text-dim" style="font-size:12px">供外部标准日志采集器投递事件使用，所有投递接口强制校验令牌</span>
        <div class="am-flex-1" />
        <el-button type="primary" :icon="'Plus'" @click="dialog = true">创建令牌</el-button>
      </div>

      <el-table :data="list" v-loading="loading" size="small">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="名称" width="180" />
        <el-table-column label="项目" width="160">
          <template #default="{ row }">{{ row.project?.name || '-' }}</template>
        </el-table-column>
        <el-table-column prop="prefix" label="令牌前缀" width="150" />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '禁用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="IP 白名单" min-width="160">
          <template #default="{ row }">{{ row.allow_cidr || '不限制' }}</template>
        </el-table-column>
        <el-table-column label="有效期" width="170">
          <template #default="{ row }">{{ row.expires_at ? formatTime(row.expires_at) : '永久' }}</template>
        </el-table-column>
        <el-table-column label="最近使用" width="170">
          <template #default="{ row }">{{ formatTime(row.last_used_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="230" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="showCurl(row)">接入示例</el-button>
            <el-button link type="primary" @click="toggle(row)">{{ row.enabled ? '禁用' : '启用' }}</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination style="margin-top:12px; justify-content:flex-end"
        layout="total, sizes, prev, pager, next" :total="total"
        v-model:current-page="query.page" v-model:page-size="query.page_size"
        @current-change="load" @size-change="search" />
    </div>

    <div class="am-card">
      <div class="am-toolbar"><span style="font-weight:600">投递协议</span></div>
      <div class="am-diff">
        POST /api/v1/ingest/events
        Header: X-AM-Token: &lt;令牌&gt;   （也支持 Authorization: Bearer &lt;令牌&gt;）

        {
          "source": "sentry",                 // 来源：sentry | loki | k8s | custom ...
          "level": "fatal",                   // fatal | error | warn | info
          "title": "panic: nil pointer ...",  // 必填（title 与 message 不能同时为空）
          "message": "order service panic",
          "stack": "panic: ...\n at service.go:128",
          "fingerprint": "order-nil-ctx-001", // 可选，用于去重与冷却；留空由系统生成
          "repo_hint": "shop-api",            // 可选，辅助定位仓库
          "occurred_at": "2026-09-03T17:00:00+08:00",
          "payload": { "env": "prod" }        // 任意原始负载
        }

        响应：{ "code":0, "data": { "action": "fix|ignore|dropped", "task_ids": [1], "reason": "..." } }
        批量投递：POST /api/v1/ingest/events/batch   { "events": [ ... ] }

        Sentry / Loki / Promtail 不能原样转发，需映射字段。完整示例与可选 sidecar 见仓库 docs/采集接入.md。
      </div>
    </div>

    <el-dialog v-model="dialog" title="创建项目令牌" width="520">
      <el-form :model="form" label-width="110px">
        <el-form-item label="所属项目" required>
          <el-select v-model="form.project_id" style="width:100%">
            <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="名称" required><el-input v-model="form.name" placeholder="如：loki-collector" /></el-form-item>
        <el-form-item label="IP 白名单">
          <el-input v-model="form.allow_cidr" placeholder="逗号分隔 CIDR，留空不限制" />
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
