<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar">
        <span style="font-weight:600">事件中心</span>
        <span class="am-text-dim" style="font-size:12px">生产日志采集器投递的原始事件与处置结果</span>
        <div class="am-flex-1" />
        <el-button :icon="'Refresh'" @click="load" />
      </div>

      <div class="am-toolbar">
        <el-select v-model="query.project_id" clearable placeholder="全部项目" style="width:180px" @change="load">
          <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
        <el-select v-model="query.status" clearable placeholder="全部状态" style="width:150px" @change="load">
          <el-option label="已接收" value="received" />
          <el-option label="已命中规则" value="matched" />
          <el-option label="已忽略" value="ignored" />
          <el-option label="已丢弃" value="dropped" />
        </el-select>
        <el-select v-model="query.level" clearable placeholder="全部级别" style="width:130px" @change="load">
          <el-option label="FATAL" value="fatal" />
          <el-option label="ERROR" value="error" />
          <el-option label="WARN" value="warn" />
          <el-option label="INFO" value="info" />
        </el-select>
        <el-input v-model="query.keyword" placeholder="标题 / 内容关键字" clearable style="width:220px" @keyup.enter="load" />
        <el-select v-model="query.days" style="width:120px" @change="load">
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
            <el-button link type="primary" @click="replay(row)">重放</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination style="margin-top:12px; justify-content:flex-end"
        layout="total, sizes, prev, pager, next" :total="total"
        v-model:current-page="query.page" v-model:page-size="query.page_size" @change="load" />
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

async function openDetail(row) {
  const r = await getEvent(row.id)
  detail.value = r.data || {}
  drawer.value = true
}

async function replay(row) {
  await ElMessageBox.confirm('重放会重新走一遍规则过滤并生成新的修复任务，确认继续？', '提示', { type: 'warning' })
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
