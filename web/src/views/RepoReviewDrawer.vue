<template>
  <el-drawer v-model="visible" :title="title" size="72%" destroy-on-close>
    <div class="review-layout">
      <el-form :model="form" label-width="88px" size="small">
        <el-form-item label="范围">
          <el-radio-group v-model="form.mode">
            <el-radio value="scan">扫描已合入代码</el-radio>
            <el-radio value="review">相对基线的 diff</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.mode === 'review'" label="基线 from">
          <el-select v-model="form.from" filterable allow-create default-first-option style="width:220px">
            <el-option v-for="b in fromOptions" :key="b" :label="b" :value="b" />
          </el-select>
          <span class="am-text-dim" style="margin-left:8px">对比当前分支 {{ repo?.branch || 'HEAD' }}（基线不能与当前分支相同）</span>
        </el-form-item>
        <el-form-item v-if="form.mode === 'scan'" label="路径">
          <el-select v-model="form.path" filterable allow-create default-first-option style="width:280px" placeholder="选择或输入路径">
            <el-option v-for="p in pathOptions" :key="p" :label="p" :value="p" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="starting" :disabled="running" @click="start">开始审查</el-button>
          <span v-if="job" class="am-text-dim" style="margin-left:12px">
            审查 #{{ job.id }} · {{ statusText }}
            <template v-if="elapsedText"> · 已用 {{ elapsedText }}</template>
            <template v-if="job.finding_n != null && job.status === 'success'"> · {{ job.finding_n }} 条意见</template>
          </span>
        </el-form-item>
      </el-form>
      <div class="review-history">
        <div class="review-history-head">审查记录</div>
        <div v-if="historyLoading" class="am-text-dim" style="font-size:12px">正在读取上次结果…</div>
        <div v-else-if="!history.length" class="am-text-dim" style="font-size:12px">还没有审查记录。开始一次后，关掉抽屉再打开仍能看到上次结果。</div>
        <ul v-else class="review-history-list">
          <li v-for="h in history" :key="h.id">
            <button type="button" class="review-history-item" :class="{ current: job?.id === h.id }" @click="openHistory(h)">
              <span class="review-history-id">#{{ h.id }}</span>
              <el-tag size="small" :type="statusType(h.status)">{{ statusLabel(h.status) }}</el-tag>
              <span class="am-text-dim">{{ historySummary(h) }}</span>
            </button>
          </li>
        </ul>
      </div>
      <div class="am-text-dim" style="font-size:12px;margin-bottom:8px">
        调用官方 Open Code Review CLI（`ocr`），不改代码。已合入主干的预埋问题请用「扫描已合入代码」；「相对基线的 diff」只审未合入当前分支的改动。模型与密钥来自「大模型配置中心」。
      </div>
      <div v-if="job && (running || job.progress || logLines.length)" class="review-progress" role="status" aria-live="polite">
        <div class="review-progress-now">{{ job.progress || (running ? '审查进行中…' : '') }}</div>
        <div v-if="running" class="am-text-dim" style="font-size:12px;margin-top:4px">
          路径扫描会逐文件调模型，单文件可能要一两分钟，不是卡死。超时 {{ timeoutHint }}。
        </div>
        <ol v-if="logLines.length" class="review-log">
          <li v-for="(line, i) in logLines" :key="i">{{ line }}</li>
        </ol>
      </div>
      <el-alert v-if="job?.status === 'failed'" type="error" :closable="false" :title="failTitle" style="margin-bottom:12px" />
      <el-alert v-else-if="job?.status === 'success' && !findings.length" type="info" :closable="false" title="审查完成，但没有意见。若代码里已有预埋问题，请改用「扫描已合入代码」，不要用与当前分支相同的基线做 diff。" style="margin-bottom:12px" />
      <div v-if="running && !findings.length" class="am-text-dim" style="font-size:12px;margin-bottom:8px">意见会在审查结束后列出，过程见上方日志。</div>
      <el-table :data="findings" size="small" @selection-change="onSel">
        <el-table-column type="selection" width="42" />
        <el-table-column prop="severity" label="级别" width="80">
          <template #default="{ row }">
            <el-tag size="small" :type="sevType(row.severity)">{{ row.severity || '-' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="位置" min-width="160">
          <template #default="{ row }">{{ loc(row) }}</template>
        </el-table-column>
        <el-table-column prop="rule" label="规则" width="100" show-overflow-tooltip />
        <el-table-column prop="title" label="摘要" min-width="220" show-overflow-tooltip />
        <el-table-column label="" width="72" align="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDetail(row)">详情</el-button>
          </template>
        </el-table-column>
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
    <el-dialog v-model="detailOpen" title="审查意见" width="560px" append-to-body>
      <template v-if="detail">
        <div class="detail-meta">
          <el-tag size="small" :type="sevType(detail.severity)">{{ detail.severity || '-' }}</el-tag>
          <span v-if="detail.rule" class="am-pill">{{ detail.rule }}</span>
          <span class="am-mono">{{ loc(detail) }}</span>
        </div>
        <p class="detail-title">{{ detail.title }}</p>
        <div class="detail-body">{{ detail.body || detail.title }}</div>
      </template>
      <template #footer>
        <el-button @click="detailOpen = false">关闭</el-button>
      </template>
    </el-dialog>
  </el-drawer>
</template>

<script setup>
import { computed, onUnmounted, ref, watch } from 'vue'
import { startRepoReview, getReviewJob, listRepoReviews, fixReviewJob } from '@/api'
import { ElMessage } from 'element-plus'
import { useDicts, splitCSV } from '@/composables/useDicts'
import { formatDuration } from '@/utils/format'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  repo: { type: Object, default: null }
})
const emit = defineEmits(['update:modelValue'])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})
const dict = useDicts()
const title = computed(() => (props.repo ? `审查 · ${props.repo.name}` : '审查'))
const form = ref({ mode: 'scan', from: 'main', path: '' })
const fromOptions = computed(() => {
  const extra = [props.repo?.branch, 'HEAD'].filter(Boolean)
  return [...new Set([...dict.values('git_branch'), ...extra])]
})
const pathOptions = computed(() => {
  const extra = splitCSV(props.repo?.code_paths)
  return [...new Set([...dict.values('code_path'), ...extra])]
})
const starting = ref(false)
const fixing = ref(false)
const historyLoading = ref(false)
const history = ref([])
const job = ref(null)
const selected = ref([])
const createdIds = ref([])
const detail = ref(null)
const detailOpen = ref(false)
const nowMs = ref(Date.now())
let timer = null
let tick = null

const running = computed(() => job.value && (job.value.status === 'pending' || job.value.status === 'running'))
const findings = computed(() => {
  const raw = job.value?.findings
  return Array.isArray(raw) ? raw : []
})
const logLines = computed(() => {
  const raw = job.value?.logs
  if (!Array.isArray(raw)) return []
  return raw.filter((line) => line && !isToolNoise(line))
})
const failTitle = computed(() => humanFail(job.value?.error_msg))
const statusText = computed(() => {
  const s = job.value?.status
  return ({ pending: '排队', running: '审查中', success: '完成', failed: '失败' })[s] || s || ''
})
const elapsedText = computed(() => {
  if (!job.value) return ''
  if (job.value.duration_ms) return formatDuration(job.value.duration_ms)
  if (!running.value || !job.value.started_at) return ''
  const ms = nowMs.value - new Date(job.value.started_at).getTime()
  if (!Number.isFinite(ms) || ms < 0) return ''
  return formatDuration(ms)
})
const timeoutHint = computed(() => '约 10 分钟')

watch(() => [visible.value, props.repo?.id], async () => {
  await dict.load()
  stopPoll()
  selected.value = []
  createdIds.value = []
  detail.value = null
  detailOpen.value = false
  if (!visible.value || !props.repo) {
    job.value = null
    history.value = []
    return
  }
  const paths = splitCSV(props.repo.code_paths)
  form.value = {
    mode: 'scan',
    from: defaultFrom(props.repo.branch),
    path: paths[0] || (String(props.repo.language).toLowerCase() === 'java' ? 'src/' : 'internal/')
  }
  await loadHistory(true)
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
function openDetail(row) {
  detail.value = row
  detailOpen.value = true
}
function isToolNoise(line) {
  const s = String(line || '').replace(/^\[ocr\]\s*/, '').trim()
  return /^[▶✔]/.test(s) || /^(full-scan:|estimated cost:|scan dispatch:|scan dedup)/.test(s)
}
function humanFail(msg) {
  const s = String(msg || '').trim()
  if (!s) return '审查失败'
  if (/exit( status)? -1|deadline|timed out|超时/i.test(s)) return '审查超时。已扫过的文件意见会尽量保留；可缩小路径后重试。'
  const first = s.split('\n').find((ln) => ln.trim() && !isToolNoise(ln)) || s
  return first.length > 180 ? first.slice(0, 180) + '…' : first
}

function defaultFrom(branch) {
  const b = String(branch || '').trim()
  if (b && b !== 'main') return 'main'
  if (b === 'main') return 'master'
  return 'main'
}

function statusLabel(s) {
  return ({ pending: '排队', running: '审查中', success: '完成', failed: '失败' })[s] || s || '-'
}
function statusType(s) {
  if (s === 'success') return 'success'
  if (s === 'failed') return 'danger'
  if (s === 'running' || s === 'pending') return 'warning'
  return 'info'
}
function historySummary(h) {
  const bits = []
  bits.push(h.mode === 'scan' ? (h.path || '扫描') : `${h.from_ref || '-'} → ${h.to_ref || '-'}`)
  if (h.status === 'success') bits.push(`${h.finding_n || 0} 条意见`)
  if (h.status === 'failed' && h.error_msg) bits.push(humanFail(h.error_msg))
  if (h.created_at) bits.push(String(h.created_at).replace('T', ' ').slice(0, 19))
  return bits.join(' · ')
}

function applyJob(next) {
  job.value = next
  if (next?.status === 'pending' || next?.status === 'running') poll()
  else stopPoll()
}

async function loadHistory(selectLatest) {
  if (!props.repo?.id) return
  historyLoading.value = true
  try {
    const r = await listRepoReviews(props.repo.id)
    history.value = Array.isArray(r.data) ? r.data : []
    if (selectLatest && history.value.length) openHistory(history.value[0])
    else if (selectLatest) job.value = null
  } finally {
    historyLoading.value = false
  }
}

function openHistory(row) {
  if (!row?.id) return
  applyJob(row)
  if (row.mode === 'scan') {
    form.value.mode = 'scan'
    if (row.path) form.value.path = row.path
  } else {
    form.value.mode = 'review'
    if (row.from_ref) form.value.from = row.from_ref
  }
}

async function start() {
  if (!props.repo?.id) return
  if (form.value.mode === 'review') {
    const from = String(form.value.from || '').trim()
    const to = String(props.repo.branch || 'main').trim()
    if (from.toLowerCase() === to.toLowerCase()) {
      ElMessage.error('基线不能与当前分支相同，否则没有 diff。预埋在主干上的问题请改用「扫描已合入代码」。')
      return
    }
  }
  starting.value = true
  createdIds.value = []
  selected.value = []
  try {
    const body = { mode: form.value.mode }
    if (form.value.mode === 'review') body.from = form.value.from
    if (form.value.mode === 'scan') body.path = form.value.path
    const r = await startRepoReview(props.repo.id, body)
    applyJob(r.data)
    await loadHistory(false)
  } finally {
    starting.value = false
  }
}

function poll() {
  stopPoll()
  tick = setInterval(() => { nowMs.value = Date.now() }, 1000)
  timer = setInterval(async () => {
    if (!job.value?.id) return
    try {
      const r = await getReviewJob(job.value.id)
      job.value = r.data
      nowMs.value = Date.now()
      const i = history.value.findIndex((x) => x.id === r.data.id)
      if (i >= 0) history.value[i] = r.data
      else history.value = [r.data, ...history.value]
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
  if (tick) {
    clearInterval(tick)
    tick = null
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
onUnmounted(stopPoll)
</script>

<style scoped>
.review-layout {
  min-height: 280px;
}
.review-history {
  margin: 0 0 14px;
  padding: 12px 14px;
  border: 1px solid var(--am-border);
  border-radius: 10px;
  background: var(--am-bg-elevated);
}
.review-history-head {
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
}
.review-history-list {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: 148px;
  overflow-y: auto;
}
.review-history-item {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  width: 100%;
  text-align: left;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 8px;
  color: var(--am-text);
  padding: 6px 8px;
  cursor: pointer;
  font: inherit;
}
.review-history-item:hover,
.review-history-item.current {
  border-color: var(--am-border);
  background: var(--am-bg-inset);
}
.review-history-id {
  font-family: 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
}
.review-progress {
  border: 1px solid var(--am-border);
  background: var(--am-bg-inset);
  border-radius: 10px;
  padding: 12px 14px;
  margin-bottom: 12px;
}
.review-progress-now {
  color: var(--am-text);
  font-size: 13px;
  font-weight: 600;
}
.review-log {
  margin: 10px 0 0;
  padding-left: 18px;
  max-height: 180px;
  overflow-y: auto;
  color: var(--am-text-dim);
  font-size: 12px;
  line-height: 1.6;
}
.review-log li + li { margin-top: 2px; }
.detail-meta { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; margin-bottom: 10px; color: var(--am-text-dim); font-size: 12px; }
.detail-title { margin: 0 0 10px; font-weight: 600; color: var(--am-text); }
.detail-body { white-space: pre-wrap; color: var(--am-text); font-size: 13px; line-height: 1.65; }
</style>
