<template>
  <el-drawer v-model="visible" :title="title" size="86%" destroy-on-close class="review-drawer">
    <div class="rv">
      <header class="rv-bar">
        <el-radio-group v-model="form.mode" class="am-seg" aria-label="审查范围">
          <el-radio-button value="scan">扫描已合入代码</el-radio-button>
          <el-radio-button value="review">相对基线的 diff</el-radio-button>
        </el-radio-group>
        <el-select
          v-if="form.mode === 'scan'"
          v-model="form.path"
          filterable
          allow-create
          default-first-option
          class="rv-bar__field"
          placeholder="路径，留空则扫整个仓库"
        >
          <el-option v-for="p in pathOptions" :key="p" :label="p" :value="p" />
        </el-select>
        <el-select
          v-else
          v-model="form.from"
          filterable
          allow-create
          default-first-option
          class="rv-bar__field"
          placeholder="基线分支"
        >
          <el-option v-for="b in fromOptions" :key="b" :label="b" :value="b" />
        </el-select>
        <el-button type="primary" :loading="starting" :disabled="running" @click="start">开始审查</el-button>
      </header>
      <p class="rv-hint">
        <template v-if="form.mode === 'scan'">扫已合入代码里的预埋问题。不改仓库。</template>
        <template v-else>只审相对 {{ repo?.branch || 'HEAD' }} 的未合入改动，基线不能与当前分支相同。</template>
      </p>

      <div class="rv-shell">
        <aside class="rv-rail" aria-label="审查记录">
          <div class="rv-rail__head">记录</div>
          <div v-if="historyLoading" class="rv-muted">正在读取…</div>
          <div v-else-if="!history.length" class="rv-muted">还没有记录。跑完一次后关掉再开仍能看到。</div>
          <ul v-else class="rv-rail__list">
            <li v-for="h in history" :key="h.id">
              <button
                type="button"
                class="rv-hist"
                :class="{ 'is-current': job?.id === h.id }"
                @click="openHistory(h)"
              >
                <span class="rv-hist__row">
                  <span class="am-mono">#{{ h.id }}</span>
                  <span class="rv-dot" :class="'is-' + (h.status || 'info')" :title="statusLabel(h.status)" />
                  <span class="rv-hist__status">{{ statusLabel(h.status) }}</span>
                </span>
                <span class="rv-hist__sum">{{ historyLead(h) }}</span>
                <span class="rv-hist__time">{{ historyWhen(h) }}</span>
              </button>
            </li>
          </ul>
        </aside>

        <section class="rv-main" aria-label="审查结果">
          <div v-if="job" class="rv-status" role="status" aria-live="polite">
            <span class="rv-status__label">{{ statusText || '—' }}</span>
            <span v-if="elapsedText" class="rv-muted">{{ elapsedText }}</span>
            <span v-if="job.status === 'success'" class="rv-status__count">{{ findings.length }} 条意见</span>
            <span v-if="running" class="rv-muted">单文件可能要一两分钟，超时 {{ timeoutHint }}</span>
          </div>

          <div v-if="running && (job.progress || logLines.length)" class="rv-live">
            <div class="rv-live__now">{{ job.progress || '审查进行中…' }}</div>
            <ol v-if="logLines.length" class="rv-log">
              <li v-for="(line, i) in logLines" :key="i">{{ line }}</li>
            </ol>
          </div>

          <div v-if="job?.status === 'failed'" class="rv-banner is-fail">{{ failTitle }}</div>
          <div v-else-if="job?.status === 'success' && !findings.length" class="rv-banner is-ok">
            没有意见。主干上的预埋问题请改用「扫描已合入代码」。
          </div>
          <div v-else-if="!job && !historyLoading" class="am-empty">
            <div class="am-empty__title">还没有这次审查</div>
            <div class="am-empty__desc">选范围后点「开始审查」。意见会列在这里，点一行看全文。</div>
          </div>
          <div v-else-if="running && !findings.length" class="rv-muted rv-wait">意见会在结束后列出。</div>

          <div v-if="findings.length" class="rv-split" :class="{ 'is-open': detailOpen && detail }">
            <div class="rv-list" role="list">
              <div
                v-for="row in findings"
                :key="findingKey(row)"
                class="rv-item"
                :class="{ 'is-open': findingKey(detail) === findingKey(row) && detailOpen }"
                role="listitem"
              >
                <el-checkbox
                  :model-value="isSelected(row)"
                  class="rv-item__check"
                  @click.stop
                  @change="(on) => toggleOne(row, on)"
                />
                <button type="button" class="rv-item__hit" @click="openDetail(row)">
                  <span class="rv-item__body">
                    <span class="rv-item__title">
                      <span class="rv-item__sev" :class="'is-' + sevTone(row.severity)">{{ sevLabel(row.severity) }}</span>
                      {{ row.title || '未命名意见' }}
                    </span>
                    <span class="rv-item__meta">
                      <span class="am-mono">{{ loc(row) }}</span>
                      <span v-if="row.rule" class="rv-item__rule">{{ row.rule }}</span>
                    </span>
                  </span>
                </button>
              </div>
            </div>
            <article v-if="detailOpen && detail" class="rv-detail" aria-label="意见全文">
              <div class="rv-detail__head">
                <span class="rv-item__sev" :class="'is-' + sevTone(detail.severity)">{{ sevLabel(detail.severity) }}</span>
                <span v-if="detail.rule" class="rv-item__rule">{{ detail.rule }}</span>
                <span class="am-mono">{{ loc(detail) }}</span>
              </div>
              <h4 class="rv-detail__title">{{ detail.title }}</h4>
              <div class="rv-detail__body">{{ detail.body || detail.title }}</div>
            </article>
          </div>

          <footer v-if="findings.length" class="rv-foot">
            <el-checkbox
              :model-value="allSelected"
              :indeterminate="someSelected"
              @change="toggleAll"
            >全选 {{ selected.length }}/{{ findings.length }}</el-checkbox>
            <el-button type="primary" :loading="fixing" :disabled="!selected.length" @click="fixSelected">
              用平台修复
            </el-button>
            <span class="rv-hint">始终半自动，补丁需人工确认再推送。</span>
            <span v-if="createdIds.length" class="rv-tasks">
              已创建
              <el-button v-for="tid in createdIds" :key="tid" link type="primary" @click="$router.push(`/tasks/${tid}`)">#{{ tid }}</el-button>
            </span>
          </footer>
        </section>
      </div>
    </div>
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
function sevTone(s) {
  if (s === 'critical' || s === 'high') return 'high'
  if (s === 'medium') return 'mid'
  return 'low'
}
function sevLabel(s) {
  return s || 'info'
}
function findingKey(row) {
  if (!row) return ''
  return row.key || `${row.path || ''}:${row.line || 0}:${row.title || ''}`
}
function openDetail(row) {
  if (findingKey(detail.value) === findingKey(row) && detailOpen.value) {
    detailOpen.value = false
    return
  }
  detail.value = row
  detailOpen.value = true
}
const allSelected = computed(() => findings.value.length > 0 && selected.value.length === findings.value.length)
const someSelected = computed(() => selected.value.length > 0 && !allSelected.value)
function isSelected(row) {
  const k = findingKey(row)
  return selected.value.some((x) => findingKey(x) === k)
}
function toggleOne(row, on) {
  const k = findingKey(row)
  if (on) {
    if (!isSelected(row)) selected.value = [...selected.value, row]
    return
  }
  selected.value = selected.value.filter((x) => findingKey(x) !== k)
}
function toggleAll(on) {
  selected.value = on ? [...findings.value] : []
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
function historyLead(h) {
  if (h.status === 'success') return `${h.finding_n || 0} 条 · ${h.mode === 'scan' ? (h.path || '扫描') : `${h.from_ref || '-'} → ${h.to_ref || '-'}`}`
  if (h.status === 'failed' && h.error_msg) return humanFail(h.error_msg)
  return h.mode === 'scan' ? (h.path || '扫描') : `${h.from_ref || '-'} → ${h.to_ref || '-'}`
}
function historyWhen(h) {
  if (!h.created_at) return ''
  return String(h.created_at).replace('T', ' ').slice(0, 16)
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
  selected.value = []
  createdIds.value = []
  detail.value = null
  detailOpen.value = false
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
  detail.value = null
  detailOpen.value = false
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
.rv {
  display: flex;
  flex-direction: column;
  gap: 8px;
  height: 100%;
  min-height: 0;
}
.rv-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.rv-bar__field { width: 220px; max-width: 100%; }
.rv-hint {
  margin: 0;
  font-size: var(--am-font-sm);
  line-height: 1.6;
  color: var(--am-text-dim);
  max-width: 72ch;
}
.rv-shell {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  gap: 16px;
  flex: 1;
  min-height: 360px;
}
.rv-rail {
  border-right: 1px solid var(--am-border);
  padding-right: 12px;
  min-width: 0;
}
.rv-rail__head {
  font-size: var(--am-font-xs);
  letter-spacing: .04em;
  text-transform: uppercase;
  color: var(--am-text-dim);
  margin-bottom: 8px;
}
.rv-rail__list {
  list-style: none;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  max-height: calc(100vh - 220px);
}
.rv-hist {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  text-align: left;
  padding: 10px 8px;
  margin: 0 0 2px;
  border: 0;
  border-radius: var(--am-radius-sm);
  background: transparent;
  color: var(--am-text);
  font: inherit;
  cursor: pointer;
  transition: background-color var(--am-duration) ease;
}
.rv-hist:hover { background: var(--am-bg-inset); }
.rv-hist.is-current { background: var(--am-primary-soft); }
.rv-hist:focus-visible {
  outline: 2px solid var(--am-primary);
  outline-offset: 2px;
}
.rv-hist__row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.rv-hist__status { font-size: var(--am-font-xs); color: var(--am-text-dim); }
.rv-hist__sum {
  font-size: var(--am-font-sm);
  line-height: 1.45;
  color: var(--am-text);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.rv-hist__time { font-size: var(--am-font-xs); color: var(--am-text-dim); }
.rv-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--am-text-dim);
  flex: 0 0 auto;
}
.rv-dot.is-success { background: var(--am-success); }
.rv-dot.is-failed { background: var(--am-danger); }
.rv-dot.is-running,
.rv-dot.is-pending { background: var(--am-warning); }
.rv-main {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}
.rv-status {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 8px 16px;
}
.rv-status__label {
  font-size: var(--am-font-xl);
  font-weight: 500;
  letter-spacing: -0.02em;
  text-wrap: pretty;
}
.rv-status__count { font-size: var(--am-font-sm); color: var(--am-text); }
.rv-muted { color: var(--am-text-dim); font-size: var(--am-font-sm); line-height: 1.6; }
.rv-wait { padding: 8px 0; }
.rv-live {
  background: var(--am-bg-inset);
  border: 1px solid var(--am-border);
  border-radius: var(--am-radius-sm);
  padding: 12px 14px;
}
.rv-live__now { font-size: var(--am-font-sm); font-weight: 500; }
.rv-log {
  margin: 8px 0 0;
  padding-left: 18px;
  max-height: 120px;
  overflow-y: auto;
  color: var(--am-text-dim);
  font-size: var(--am-font-xs);
  line-height: 1.6;
}
.rv-banner {
  padding: 10px 12px;
  border-radius: var(--am-radius-sm);
  font-size: var(--am-font-sm);
  line-height: 1.6;
}
.rv-banner.is-fail {
  background: color-mix(in srgb, var(--am-danger) 14%, transparent);
  color: var(--am-text);
}
.rv-banner.is-ok {
  background: var(--am-bg-inset);
  color: var(--am-text-dim);
}
.rv-split {
  display: grid;
  grid-template-columns: 1fr;
  gap: 0;
  border: 1px solid var(--am-border);
  border-radius: var(--am-radius);
  overflow: hidden;
  min-height: 280px;
  background: var(--am-bg);
}
.rv-split.is-open {
  grid-template-columns: minmax(280px, 1fr) minmax(0, 1.1fr);
}
.rv-list { overflow-y: auto; max-height: calc(100vh - 280px); }
.rv-item {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr);
  gap: 4px;
  align-items: start;
  border-bottom: 1px solid var(--am-border);
  background: transparent;
  transition: background-color var(--am-duration) ease;
}
.rv-item:hover,
.rv-item.is-open { background: var(--am-bg-inset); }
.rv-item__check { margin: 12px 0 0 10px; }
.rv-item__hit {
  display: block;
  width: 100%;
  text-align: left;
  padding: 12px 14px 12px 4px;
  border: 0;
  background: transparent;
  color: var(--am-text);
  font: inherit;
  cursor: pointer;
}
.rv-item__hit:focus-visible {
  outline: 2px solid var(--am-primary);
  outline-offset: -2px;
}
.rv-item__sev {
  font-size: var(--am-font-xs);
  letter-spacing: .04em;
  text-transform: uppercase;
  color: var(--am-text-dim);
  margin-right: 8px;
}
.rv-item__sev.is-high { color: var(--am-danger); }
.rv-item__sev.is-mid { color: var(--am-warning); }
.rv-item__body { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.rv-item__title {
  font-size: var(--am-font-md);
  line-height: 1.45;
  text-wrap: pretty;
}
.rv-item__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  color: var(--am-text-dim);
  font-size: var(--am-font-xs);
}
.rv-item__rule { color: var(--am-text-dim); }
.rv-detail {
  border-left: 1px solid var(--am-border);
  background: var(--am-bg-elevated);
  padding: 16px 18px;
  overflow-y: auto;
  max-height: calc(100vh - 280px);
}
.rv-detail__head {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  margin-bottom: 10px;
  color: var(--am-text-dim);
  font-size: var(--am-font-xs);
}
.rv-detail__title {
  margin: 0 0 12px;
  font-size: var(--am-font-lg);
  font-weight: 500;
  letter-spacing: -0.02em;
  line-height: 1.4;
  text-wrap: pretty;
}
.rv-detail__body {
  white-space: pre-wrap;
  font-size: var(--am-font-md);
  line-height: 1.65;
  max-width: 68ch;
}
.rv-foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  padding-top: 8px;
  position: sticky;
  bottom: 0;
  background: var(--am-bg-elevated);
  z-index: var(--am-z-overlay);
}
.rv-tasks { display: inline-flex; flex-wrap: wrap; align-items: center; gap: 4px; font-size: var(--am-font-sm); color: var(--am-text-dim); }

@media (max-width: 1023.98px) {
  .rv-shell { grid-template-columns: 1fr; }
  .rv-rail {
    border-right: 0;
    border-bottom: 1px solid var(--am-border);
    padding-right: 0;
    padding-bottom: 8px;
  }
  .rv-rail__list { max-height: 140px; display: flex; gap: 4px; overflow-x: auto; overflow-y: hidden; }
  .rv-rail__list li { flex: 0 0 200px; }
  .rv-split.is-open { grid-template-columns: 1fr; }
  .rv-detail { border-left: 0; border-top: 1px solid var(--am-border); }
  .rv-list, .rv-detail { max-height: none; }
}
@media (max-width: 767.98px) {
  .rv-bar__field { width: 100%; }
  .rv-status__label { font-size: var(--am-font-xl); }
  .rv-bar :deep(.am-seg) { width: 100%; }
  .rv-bar :deep(.am-seg .el-radio-button) { flex: 1 1 50%; }
  .rv-bar :deep(.am-seg .el-radio-button__inner) { width: 100%; }
  .rv-foot .rv-hint { width: 100%; }
}
</style>
