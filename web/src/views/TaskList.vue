<template>
  <div class="am-page">
    <div class="am-page-head">
      <div>
        <div class="am-page-head__crumb">首页</div>
        <h1 class="am-page-head__title">修复流程</h1>
        <p class="am-page-head__desc">
          每个流程对应一次 dsh 修复：准备隔离工作区 → 改代码 → 产出补丁 →（半自动）人工确认 → 提交推送。
        </p>
      </div>
    </div>

    <div class="am-card">
      <div class="am-toolbar">
        <el-select v-model="query.project_id" clearable placeholder="全部项目" style="width:180px" @change="search">
          <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
        <el-select v-model="query.status" clearable placeholder="全部状态" style="width:150px" @change="search">
          <el-option v-for="(v, k) in STATUS_META" :key="k" :label="v.label" :value="k" />
        </el-select>
        <el-select v-model="query.mode" clearable placeholder="全部模式" style="width:130px" @change="search">
          <el-option label="全自动" value="auto" />
          <el-option label="半自动" value="semi" />
        </el-select>
        <el-input v-model="query.keyword" placeholder="摘要 / 分支 / 提交" clearable style="width:200px" @keyup.enter="search" @clear="search" />
        <el-button type="primary" :icon="'Search'" @click="search">查询</el-button>
        <div class="am-flex-1" />
        <el-button :icon="'Refresh'" @click="load" />
      </div>

      <el-table :data="list" v-loading="loading" size="small" @row-click="row => $router.push('/tasks/' + row.id)">
        <el-table-column label="流程" width="70">
          <template #default="{ row }"><span class="am-mono">#{{ row.id }}</span></template>
        </el-table-column>
        <el-table-column label="事件号" width="168">
          <template #default="{ row }">
            <router-link
              v-if="row.event"
              class="am-link am-mono"
              :to="{ path: '/events', query: { code: eventCode(row.event) } }"
              @click.stop
            >
              {{ eventCode(row.event) }}
            </router-link>
            <span v-else class="am-text-faint">手动触发</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="116">
          <template #default="{ row }">
            <el-tag size="small" :type="STATUS_META[row.status]?.type">{{ STATUS_META[row.status]?.label }}</el-tag>
            <div class="am-text-dim am-hint">{{ STAGE_LABEL[row.stage] || '' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="触发事件" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <div>{{ row.event?.title || '手动触发' }}</div>
            <div class="am-text-dim am-hint">{{ row.project?.name || '-' }} · {{ row.repo?.name || '-' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="模式" width="84">
          <template #default="{ row }">
            <el-tag size="small" effect="plain" :type="row.mode === 'auto' ? 'danger' : 'warning'">
              {{ row.mode === 'auto' ? '全自动' : '半自动' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="模型" width="120">
          <template #default="{ row }">{{ row.model?.name || row.dsh_model || '-' }}</template>
        </el-table-column>
        <el-table-column label="耗时" width="72">
          <template #default="{ row }">{{ formatDuration(row.duration_ms) }}</template>
        </el-table-column>
        <el-table-column label="创建时间" width="132">
          <template #default="{ row }">{{ formatTimeShort(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click.stop="$router.push('/tasks/' + row.id)">详情</el-button>
            <el-button
              link
              type="primary"
              :disabled="!canRetry(row)"
              :loading="retryingId === row.id"
              :title="canRetry(row) ? (canResumePush(row) ? '重试推送（不重跑 dsh）' : '重试修复') : retryDisabledHint(row)"
              :aria-disabled="!canRetry(row)"
              @click.stop="retry(row)"
            >{{ canResumePush(row) ? '续推' : '重试' }}</el-button>
            <el-button
              link
              :disabled="!canCancel(row)"
              :loading="cancellingId === row.id"
              @click.stop="cancel(row)"
            >取消</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination class="am-pagination"
        layout="total, sizes, prev, pager, next" :total="total"
        v-model:current-page="query.page" v-model:page-size="query.page_size"
        @current-change="load" @size-change="search" />
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { listTasks, retryTask, cancelTask, listProjects } from '@/api'
import { STATUS_META, STAGE_LABEL, formatDuration, formatTimeShort, eventCode } from '@/utils/format'
import { ElMessage } from 'element-plus'

const router = useRouter()
const list = ref([])
const projects = ref([])
const total = ref(0)
const loading = ref(false)
// 行级写操作 in-flight 守卫：双击不再产生第二个重试任务 / 重复取消
const retryingId = ref(0)
const cancellingId = ref(0)
const query = reactive({ page: 1, page_size: 20, project_id: '', status: '', mode: '', keyword: '' })

async function load() {
  loading.value = true
  try {
    const r = await listTasks(query)
    list.value = r.data?.list || []
    total.value = r.data?.total || 0
  } finally { loading.value = false }
}

// 筛选条件/每页条数变化后必须回到第 1 页，否则停留在旧页码可能拿到空列表
function search() {
  query.page = 1
  return load()
}

function canResumePush(row) {
  return row.status === 'failed' && !!(row.patch || row.fix_commit || row.workspace)
}

function canRetry(row) {
  if (canResumePush(row)) return true
  return ['failed', 'ignored', 'rejected', 'cancelled'].includes(row.status)
}

function canCancel(row) {
  return ['pending', 'running'].includes(row.status)
}

function retryDisabledHint(row) {
  if (row.status === 'success') return '任务已成功，无需重试'
  if (['pending', 'running', 'confirming'].includes(row.status)) return '任务仍在进行中，不能重试'
  return '当前状态不能重试'
}

async function retry(row) {
  if (!canRetry(row) || retryingId.value || cancellingId.value) return
  retryingId.value = row.id
  try {
    const r = await retryTask(row.id)
    if (r.data?.resume) {
      ElMessage.success('正在重试提交并推送，不会重新跑 dsh')
    } else {
      ElMessage.success('已创建重试任务 #' + r.data.id)
    }
    await load()
  } finally { retryingId.value = 0 }
}

async function cancel(row) {
  if (!canCancel(row) || cancellingId.value || retryingId.value) return
  cancellingId.value = row.id
  try {
    await cancelTask(row.id)
    ElMessage.success('已取消')
    await load()
  } finally { cancellingId.value = 0 }
}

onMounted(async () => {
  const p = await listProjects({ page_size: 100 })
  projects.value = p.data?.list || []
  load()
})
</script>
