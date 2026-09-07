<template>
  <el-drawer v-model="visible" :title="title" size="72%" destroy-on-close>
    <div class="review-layout">
      <el-form :model="form" label-width="88px" size="small">
        <el-form-item label="范围">
          <el-radio-group v-model="form.mode">
            <el-radio value="review">相对基线的 diff</el-radio>
            <el-radio value="scan">指定路径</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.mode === 'review'" label="基线 from">
          <el-input v-model="form.from" placeholder="例如 main" style="width:220px" />
          <span class="am-text-dim" style="margin-left:8px">对比当前分支 {{ repo?.branch || 'HEAD' }}</span>
        </el-form-item>
        <el-form-item v-if="form.mode === 'scan'" label="路径">
          <el-input v-model="form.path" placeholder="例如 internal/" style="width:280px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="starting" :disabled="running" @click="start">开始审查</el-button>
          <span v-if="job" class="am-text-dim" style="margin-left:12px">
            审查 #{{ job.id }} · {{ statusText }}
            <template v-if="job.finding_n != null && job.status === 'success'"> · {{ job.finding_n }} 条意见</template>
          </span>
        </el-form-item>
      </el-form>
      <div class="am-text-dim" style="font-size:12px;margin-bottom:8px">
        调用官方 Open Code Review CLI（`ocr`），不改代码。有问题请勾选后用平台半自动修复。
      </div>
      <el-alert v-if="job?.status === 'failed'" type="error" :closable="false" :title="job.error_msg || '审查失败'" style="margin-bottom:12px" />
      <el-table :data="findings" size="small" v-loading="running" @selection-change="onSel">
        <el-table-column type="selection" width="42" />
        <el-table-column prop="severity" label="级别" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="sevType(row.severity)">{{ row.severity || '-' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="位置" min-width="180">
          <template #default="{ row }">{{ loc(row) }}</template>
        </el-table-column>
        <el-table-column prop="rule" label="规则" width="120" show-overflow-tooltip />
        <el-table-column prop="title" label="意见" min-width="220" show-overflow-tooltip />
      </el-table>
      <div v-if="selected.length" class="am-toolbar" style="margin-top:12px">
        <el-button type="primary" :loading="fixing" @click="fixSelected">用平台修复（{{ selected.length }}）</el-button>
        <span class="am-text-dim" style="font-size:12px">始终半自动：dsh 产出补丁后需人工确认再推送</span>
      </div>
      <div v-if="createdIds.length" class="am-text-dim" style="margin-top:8px;font-size:12px">
        已创建任务
        <el-button v-for="tid in createdIds" :key="tid" link type="primary" @click="$router.push(`/tasks/${tid}`)">#{{ tid }}</el-button>
      </div>
    </div>
  </el-drawer>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { startRepoReview, getReviewJob, fixReviewJob } from '@/api'
import { ElMessage } from 'element-plus'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  repo: { type: Object, default: null }
})
const emit = defineEmits(['update:modelValue'])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})
const title = computed(() => (props.repo ? `审查 · ${props.repo.name}` : '审查'))
const form = ref({ mode: 'review', from: 'main', path: '' })
const starting = ref(false)
const fixing = ref(false)
const job = ref(null)
const selected = ref([])
const createdIds = ref([])
let timer = null

const running = computed(() => job.value && (job.value.status === 'pending' || job.value.status === 'running'))
const findings = computed(() => {
  const raw = job.value?.findings
  return Array.isArray(raw) ? raw : []
})
const statusText = computed(() => {
  const s = job.value?.status
  return ({ pending: '排队', running: '审查中', success: '完成', failed: '失败' })[s] || s || ''
})

watch(() => [visible.value, props.repo?.id], () => {
  stopPoll()
  job.value = null
  selected.value = []
  createdIds.value = []
  if (visible.value && props.repo) {
    form.value = {
      mode: 'review',
      from: 'main',
      path: props.repo.code_paths ? String(props.repo.code_paths).split(',')[0].trim() : ''
    }
  }
})

function loc(row) {
  if (!row?.path) return '-'
  return row.line ? `${row.path}:${row.line}` : row.path
}
function sevType(s) {
  if (s === 'critical' || s === 'high') return 'danger'
  if (s === 'medium') return 'warning'
  return 'info'
}
function onSel(rows) {
  selected.value = rows || []
}

async function start() {
  if (!props.repo?.id) return
  starting.value = true
  createdIds.value = []
  selected.value = []
  try {
    const body = { mode: form.value.mode }
    if (form.value.mode === 'review') body.from = form.value.from
    if (form.value.mode === 'scan') body.path = form.value.path
    const r = await startRepoReview(props.repo.id, body)
    job.value = r.data
    poll()
  } finally {
    starting.value = false
  }
}

function poll() {
  stopPoll()
  timer = setInterval(async () => {
    if (!job.value?.id) return
    try {
      const r = await getReviewJob(job.value.id)
      job.value = r.data
      if (job.value.status === 'success' || job.value.status === 'failed') stopPoll()
    } catch {
      stopPoll()
    }
  }, 1500)
}

function stopPoll() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

async function fixSelected() {
  if (!job.value?.id || !selected.value.length) return
  fixing.value = true
  try {
    const r = await fixReviewJob(job.value.id, { keys: selected.value.map((x) => x.key) })
    createdIds.value = r.data?.task_ids || []
    ElMessage.success(`已创建 ${createdIds.value.length} 个半自动修复任务`)
  } finally {
    fixing.value = false
  }
}

watch(visible, (v) => { if (!v) stopPoll() })
</script>

<style scoped>
.review-layout {
  min-height: 280px;
}
</style>
