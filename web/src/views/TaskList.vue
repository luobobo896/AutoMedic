<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar">
        <el-select v-model="query.project_id" clearable placeholder="全部项目" style="width:180px" @change="load">
          <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
        <el-select v-model="query.status" clearable placeholder="全部状态" style="width:150px" @change="load">
          <el-option v-for="(v, k) in STATUS_META" :key="k" :label="v.label" :value="k" />
        </el-select>
        <el-select v-model="query.mode" clearable placeholder="全部模式" style="width:130px" @change="load">
          <el-option label="全自动" value="auto" />
          <el-option label="半自动" value="semi" />
        </el-select>
        <el-input v-model="query.keyword" placeholder="摘要 / 分支 / 提交" clearable style="width:200px" @keyup.enter="load" />
        <div class="am-flex-1" />
        <el-button :icon="'Refresh'" @click="load" />
      </div>

      <el-table :data="list" v-loading="loading" size="small" @row-click="row => $router.push('/tasks/' + row.id)">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag size="small" :type="STATUS_META[row.status]?.type">{{ STATUS_META[row.status]?.label }}</el-tag>
            <div class="am-text-dim" style="font-size:11px">{{ STAGE_LABEL[row.stage] || '' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="项目 / 仓库" width="190">
          <template #default="{ row }">
            <div>{{ row.project?.name || '-' }}</div>
            <div class="am-text-dim" style="font-size:11px">{{ row.repo?.name || '-' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="触发事件" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">{{ row.event?.title || '手动触发' }}</template>
        </el-table-column>
        <el-table-column label="模式" width="90">
          <template #default="{ row }">
            <el-tag size="small" effect="plain" :type="row.mode === 'auto' ? 'danger' : 'warning'">
              {{ row.mode === 'auto' ? '全自动' : '半自动' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="模型" width="150">
          <template #default="{ row }">{{ row.model?.name || row.dsh_model || '-' }}</template>
        </el-table-column>
        <el-table-column label="耗时" width="90">
          <template #default="{ row }">{{ formatDuration(row.duration_ms) }}</template>
        </el-table-column>
        <el-table-column label="创建时间" width="170">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click.stop="$router.push('/tasks/' + row.id)">详情</el-button>
            <el-button link type="primary" @click.stop="retry(row)">重试</el-button>
            <el-button link @click.stop="cancel(row)">取消</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination style="margin-top:12px; justify-content:flex-end"
        layout="total, sizes, prev, pager, next" :total="total"
        v-model:current-page="query.page" v-model:page-size="query.page_size" @change="load" />
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { listTasks, retryTask, cancelTask, listProjects } from '@/api'
import { STATUS_META, STAGE_LABEL, formatDuration, formatTime } from '@/utils/format'
import { ElMessage } from 'element-plus'

const list = ref([])
const projects = ref([])
const total = ref(0)
const loading = ref(false)
const query = reactive({ page: 1, page_size: 20, project_id: '', status: '', mode: '', keyword: '' })

async function load() {
  loading.value = true
  try {
    const r = await listTasks(query)
    list.value = r.data?.list || []
    total.value = r.data?.total || 0
  } finally { loading.value = false }
}

async function retry(row) {
  const r = await retryTask(row.id)
  ElMessage.success('已创建重试任务 #' + r.data.id)
  load()
}

async function cancel(row) {
  await cancelTask(row.id)
  ElMessage.success('已取消')
  load()
}

onMounted(async () => {
  const p = await listProjects({ page_size: 100 })
  projects.value = p.data?.list || []
  load()
})
</script>
