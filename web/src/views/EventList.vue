<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar">
        <span style="font-weight:600">事件中心</span>
        <span class="am-text-dim" style="font-size:12px">同一指纹合并为一条。修复成功后状态同步为「已修复」，不能再重放。</span>
        <div class="am-flex-1" />
        <el-button :icon="'Refresh'" @click="load" />
      </div>

      <div class="am-toolbar">
        <el-select v-model="query.project_id" clearable placeholder="全部项目" style="width:180px" @change="search">
          <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
        <el-select v-model="query.status" clearable placeholder="全部状态" style="width:150px" @change="search">
          <el-option label="已接收" value="received" />
          <el-option label="修复中" value="fixing" />
          <el-option label="已修复" value="fixed" />
          <el-option label="修复失败" value="failed" />
          <el-option label="已忽略" value="ignored" />
          <el-option label="已丢弃" value="dropped" />
        </el-select>
        <el-select v-model="query.level" clearable placeholder="全部级别" style="width:130px" @change="search">
          <el-option label="FATAL" value="fatal" />
          <el-option label="ERROR" value="error" />
          <el-option label="WARN" value="warn" />
          <el-option label="INFO" value="info" />
        </el-select>
        <el-input v-model="query.keyword" placeholder="标题 / 内容关键字" clearable style="width:220px" @keyup.enter="search" @clear="search" />
        <el-select v-model="query.days" style="width:120px" @change="search">
          <el-option label="近 24 小时" :value="1" />
          <el-option label="近 7 天" :value="7" />
          <el-option label="近 30 天" :value="30" />
          <el-option label="全部" :value="0" />
        </el-select>
      </div>

      <el-table :data="list" v-loading="loading" size="small">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column label="级别" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="LEVEL_META[row.level]?.type">{{ LEVEL_META[row.level]?.label || row.level }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="标题" min-width="280" show-overflow-tooltip>
          <template #default="{ row }">
            <el-link type="primary" @click="openDetail(row)">{{ row.title }}</el-link>
            <el-tag v-if="row.occurrence_n > 1" size="small" effect="plain" style="margin-left:8px">{{ row.occurrence_n }} 次</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="项目" width="140">
          <template #default="{ row }">{{ row.project?.name || '-' }}</template>
        </el-table-column>
        <el-table-column prop="source" label="来源" width="110" />
        <el-table-column label="处置" width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <el-tag size="small" :type="EVENT_STATUS_META[row.status]?.type">{{ EVENT_STATUS_META[row.status]?.label }}</el-tag>
            <span class="am-text-dim" style="margin-left:6px;font-size:12px">{{ row.dispose_msg }}</span>
          </template>
        </el-table-column>
        <el-table-column label="发生时间" width="170">
          <template #default="{ row }">{{ formatTime(row.occurred_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button
              link
              type="primary"
              :disabled="!canReplay(row)"
              :title="replayHint(row)"
              @click="replay(row)"
            >重放</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination style="margin-top:12px; justify-content:flex-end"
        layout="total, sizes, prev, pager, next" :total="total"
        v-model:current-page="query.page" v-model:page-size="query.page_size"
        @current-change="load" @size-change="search" />
    </div>

    <el-drawer v-model="drawer" title="事件详情" size="60%">
      <template v-if="detail.event">
        <el-descriptions :column="2" border size="small">
          <el-descriptions-item label="ID">{{ detail.event.id }}</el-descriptions-item>
          <el-descriptions-item label="级别">{{ detail.event.level }}</el-descriptions-item>
          <el-descriptions-item label="来源">{{ detail.event.source }}</el-descriptions-item>
          <el-descriptions-item label="指纹">{{ detail.event.fingerprint }}</el-descriptions-item>
          <el-descriptions-item label="命中规则">{{ detail.event.rule?.name || '-' }}</el-descriptions-item>
          <el-descriptions-item label="发生时间">{{ formatTime(detail.event.occurred_at) }}</el-descriptions-item>
          <el-descriptions-item label="标题" :span="2">{{ detail.event.title }}</el-descriptions-item>
          <el-descriptions-item label="正文" :span="2">
            <pre style="white-space:pre-wrap;margin:0">{{ detail.event.message }}</pre>
          </el-descriptions-item>
          <el-descriptions-item label="堆栈 / 日志" :span="2">
            <div class="am-diff" style="max-height:300px">{{ detail.event.stack || '-' }}</div>
          </el-descriptions-item>
          <el-descriptions-item label="原始负载" :span="2">
            <div class="am-diff" style="max-height:200px">{{ JSON.stringify(parseJSON(detail.event.payload, {}), null, 2) }}</div>
          </el-descriptions-item>
        </el-descriptions>

        <h4 style="margin:16px 0 8px">关联修复任务</h4>
        <el-table :data="detail.tasks || []" size="small">
          <el-table-column prop="id" label="ID" width="70" />
          <el-table-column label="状态" width="110">
            <template #default="{ row }">
              <el-tag size="small" :type="STATUS_META[row.status]?.type">{{ STATUS_META[row.status]?.label }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="仓库" width="160">
            <template #default="{ row }">{{ row.repo?.name || '-' }}</template>
          </el-table-column>
          <el-table-column label="操作" width="120">
            <template #default="{ row }">
              <el-button link type="primary" @click="$router.push('/tasks/' + row.id)">查看详情</el-button>
            </template>
          </el-table-column>
        </el-table>
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { listEvents, getEvent, replayEvent, listProjects } from '@/api'
import { LEVEL_META, EVENT_STATUS_META, STATUS_META, formatTime, parseJSON } from '@/utils/format'
import { ElMessage, ElMessageBox } from 'element-plus'

const router = useRouter()
const list = ref([])
const projects = ref([])
const total = ref(0)
const loading = ref(false)
const drawer = ref(false)
const detail = ref({})
const query = reactive({ page: 1, page_size: 20, project_id: '', status: '', level: '', keyword: '', days: 0 })

async function load() {
  loading.value = true
  try {
    const r = await listEvents(query)
    list.value = r.data?.list || []
    total.value = r.data?.total || 0
  } finally { loading.value = false }
}

// 筛选条件/每页条数变化后必须回到第 1 页，否则停留在旧页码可能拿到空列表
function search() {
  query.page = 1
  return load()
}

async function openDetail(row) {
  const r = await getEvent(row.id)
  detail.value = r.data || {}
  drawer.value = true
}

function canReplay(row) {
  return !['fixed', 'fixing'].includes(row.status)
}
function replayHint(row) {
  if (row.status === 'fixed') return '已修复，不能重放'
  if (row.status === 'fixing') return '正在修复，不能重放'
  return ''
}

async function replay(row) {
  if (!canReplay(row)) return
  const ok = await ElMessageBox.confirm('重放会按同一指纹重新走规则。已修复或修复中的事件不会再开任务。', '提示', { type: 'warning' }).catch(() => false)
  if (!ok) return
  const r = await replayEvent(row.id)
  ElMessage.success(r.data?.reason || '已重放')
  load()
}

onMounted(async () => {
  const p = await listProjects({ page_size: 100 })
  projects.value = p.data?.list || []
  load()
})
</script>
