<template>
  <div class="am-page am-task-detail" v-loading="booting">
    <div class="am-toolbar">
      <el-button link type="primary" @click="$router.push('/tasks')">← 返回任务列表</el-button>
      <h3 style="margin:0">修复任务 #{{ id }}</h3>
      <el-tag :type="STATUS_META[task.status]?.type">{{ STATUS_META[task.status]?.label }}</el-tag>
      <el-tag v-if="task.stage" type="info" effect="plain">{{ STAGE_LABEL[task.stage] || task.stage }}</el-tag>
      <el-tag effect="plain" :type="task.mode === 'auto' ? 'danger' : 'warning'">
        {{ task.mode === 'auto' ? '全自动' : '半自动确认' }}
      </el-tag>
      <div class="am-flex-1" />
      <el-button v-if="task.status === 'confirming'" type="success" :icon="'Check'" @click="confirmFix">确认修复并推送</el-button>
      <el-button v-if="task.status === 'confirming'" type="danger" :icon="'Close'" @click="rejectFix">驳回</el-button>
      <el-button v-if="['failed','ignored','rejected'].includes(task.status)" :icon="'RefreshRight'" @click="retryTaskDo">重试</el-button>
      <el-button v-if="['pending','running'].includes(task.status)" :icon="'CircleClose'" @click="cancelTaskDo">取消</el-button>
      <el-button :icon="'Refresh'" @click="loadAll" />
    </div>

    <div class="task-body">
      <section class="task-process" aria-label="修复过程">
        <div class="am-card term-card">
          <div class="am-toolbar term-toolbar">
            <span class="term-title">修复过程</span>
            <el-tag v-if="wsConnected" size="small" type="success" effect="dark">实时连接</el-tag>
            <el-tag v-else size="small" type="info" effect="plain">离线（轮询）</el-tag>
            <span class="am-text-dim term-meta">{{ logs.length }} 行</span>
            <div class="am-flex-1" />
            <el-checkbox v-model="autoScroll" size="small">跟随输出</el-checkbox>
            <el-button size="small" @click="copyLogs">复制日志</el-button>
          </div>
          <div ref="termRef" class="am-terminal" @scroll="onTermScroll">
            <div v-for="(l, i) in logs" :key="l.seq || i" :class="'line-' + l.stream">
              <span class="am-text-dim">[{{ l.seq }}]</span> {{ stripANSI(l.content) }}
            </div>
            <div v-if="!logs.length" class="am-text-dim">等待 dsh 输出…</div>
          </div>
        </div>
      </section>

      <aside class="task-side" aria-label="任务信息与结果">
        <div class="am-card">
          <div class="am-toolbar"><span style="font-weight:600">任务信息</span></div>
          <el-descriptions :column="1" border size="small">
            <el-descriptions-item label="项目">{{ task.project?.name || '-' }}</el-descriptions-item>
            <el-descriptions-item label="仓库">{{ task.repo?.name || '-' }}</el-descriptions-item>
            <el-descriptions-item label="分支">
              <span class="am-mono">{{ task.branch || '-' }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="基线提交">
              <span class="am-mono">{{ (task.base_commit || '-').slice(0, 12) }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="修复提交">
              <span class="am-mono">{{ (task.fix_commit || '-').slice(0, 12) }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="工作区">
              <span class="am-mono" style="word-break:break-all">{{ task.workspace || '-' }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="模型">
              {{ task.model?.name || task.dsh_model || '-' }}
              <div class="am-text-dim" style="font-size:11px">
                provider={{ task.dsh_provider || '-' }} ctx={{ formatTokens(task.input_context) }}/{{ formatTokens(task.output_context) }}
              </div>
            </el-descriptions-item>
            <el-descriptions-item label="命中规则">{{ task.rule?.name || '-' }}</el-descriptions-item>
            <el-descriptions-item label="重试次数">{{ task.retry || 0 }}</el-descriptions-item>
            <el-descriptions-item label="开始时间">{{ formatTime(task.started_at) }}</el-descriptions-item>
            <el-descriptions-item label="结束时间">{{ formatTime(task.finished_at) }}</el-descriptions-item>
            <el-descriptions-item label="耗时">{{ formatDuration(task.duration_ms) }}</el-descriptions-item>
            <el-descriptions-item label="人工确认">
              <template v-if="task.confirmed_by">
                {{ task.confirmed_by }} @ {{ formatTime(task.confirmed_at) }}
                <div class="am-text-dim" style="font-size:11px">{{ task.confirm_note }}</div>
              </template>
              <span v-else class="am-text-dim">-</span>
            </el-descriptions-item>
          </el-descriptions>
        </div>

        <div class="am-card">
          <div class="am-toolbar"><span style="font-weight:600">修复结果</span></div>
          <el-descriptions :column="1" border size="small">
            <el-descriptions-item label="根因分析">{{ task.diagnosis || '-' }}</el-descriptions-item>
            <el-descriptions-item label="修复摘要">
              <div style="white-space:pre-wrap">{{ task.summary || '-' }}</div>
            </el-descriptions-item>
            <el-descriptions-item label="变更文件">
              <template v-if="changedFiles.length">
                <el-tag v-for="f in changedFiles" :key="f" size="small" style="margin:2px 4px 2px 0">{{ f }}</el-tag>
              </template>
              <span v-else class="am-text-dim">-</span>
            </el-descriptions-item>
            <el-descriptions-item label="变更统计">
              <pre style="margin:0;white-space:pre-wrap">{{ task.diff_stat || '-' }}</pre>
            </el-descriptions-item>
            <el-descriptions-item label="错误信息">
              <span style="color:#f7768e">{{ task.error_msg || '-' }}</span>
            </el-descriptions-item>
          </el-descriptions>
          <div class="am-toolbar" style="margin-top:12px">
            <span style="font-weight:600">补丁 Diff</span>
            <div class="am-flex-1" />
            <el-button size="small" @click="copyPatch" :disabled="!patchText">复制补丁</el-button>
          </div>
          <div class="am-diff">
            <div v-for="(l, i) in patchLines" :key="i" :class="diffClass(l)">{{ l }}</div>
            <div v-if="!patchLines.length" class="am-text-dim">暂无补丁</div>
          </div>
        </div>

        <div class="am-card">
          <div class="am-toolbar"><span style="font-weight:600">触发事件</span></div>
          <template v-if="task.event">
            <div style="margin-bottom:6px">
              <el-tag size="small" :type="LEVEL_META[task.event.level]?.type">{{ task.event.level }}</el-tag>
              <span style="margin-left:6px">{{ task.event.title }}</span>
            </div>
            <div class="am-text-dim" style="font-size:12px;margin-bottom:6px">
              {{ task.event.source }} · {{ formatTime(task.event.occurred_at) }}
            </div>
            <div class="am-diff" style="max-height:220px">{{ task.event.stack || task.event.message || '-' }}</div>
          </template>
          <span v-else class="am-text-dim">手动触发</span>
        </div>

        <div class="am-card">
          <div class="am-toolbar"><span style="font-weight:600">dsh 调用命令</span></div>
          <div class="am-diff" style="max-height:140px; font-size:11px">{{ task.dsh_cmd || '-' }}</div>
          <div class="am-text-dim" style="font-size:12px;margin-top:6px">
            退出码：{{ task.dsh_exit_code }}
          </div>
        </div>
      </aside>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getTask, taskLogs, taskPatch, confirmTask, rejectTask, retryTask, cancelTask, taskWSURL } from '@/api'
import { STATUS_META, STAGE_LABEL, LEVEL_META, formatTime, formatDuration, formatTokens, stripANSI, parseJSON, copyText } from '@/utils/format'
import { ElMessage, ElMessageBox } from 'element-plus'

const route = useRoute()
const router = useRouter()
const id = computed(() => route.params.id)
const task = ref({})
const logs = ref([])
const patchText = ref('')
const booting = ref(true)
const autoScroll = ref(true)
const wsConnected = ref(false)
const termRef = ref()

let ws = null
let timer = null
let lastSeq = 0
let logIndex = new Set()

function ended(status) {
  return ['success', 'failed', 'ignored', 'rejected', 'cancelled'].includes(status)
}

function appendLogs(rows) {
  if (!Array.isArray(rows) || !rows.length) return
  for (const row of rows) {
    const seq = Number(row.seq) || 0
    const key = seq || `${row.stream}:${row.content}`
    if (logIndex.has(key)) continue
    logIndex.add(key)
    logs.value.push(row)
    if (seq > lastSeq) lastSeq = seq
  }
  scrollBottom()
}

const changedFiles = computed(() => parseJSON(task.value.changed_files, []) || [])
const patchLines = computed(() => (patchText.value || '').split('\n'))

function diffClass(line) {
  if (line.startsWith('+') && !line.startsWith('+++')) return 'add'
  if (line.startsWith('-') && !line.startsWith('---')) return 'del'
  if (line.startsWith('@@')) return 'hunk'
  return ''
}

async function loadAll({ silent } = {}) {
  if (!silent && !task.value.id) booting.value = true
  try {
    const r = await getTask(id.value)
    task.value = r.data || {}
    const lr = await taskLogs(id.value, silent ? { after_seq: lastSeq, limit: 1000 } : { limit: 2000 })
    if (!silent) {
      logs.value = []
      logIndex = new Set()
      lastSeq = 0
    }
    appendLogs(Array.isArray(lr.data) ? lr.data : [])
    if (ended(task.value.status) || task.value.patch || task.value.diff_stat) {
      const pr = await taskPatch(id.value)
      patchText.value = pr.data?.patch || patchText.value
    }
    if (ended(task.value.status)) {
      stopWS()
      stopPolling()
    } else {
      startWS()
      startPolling()
    }
  } finally {
    booting.value = false
  }
}

function startWS() {
  if (ws || ended(task.value.status)) return
  try {
    ws = new WebSocket(taskWSURL(id.value))
    ws.onopen = () => { wsConnected.value = true }
    ws.onclose = () => {
      wsConnected.value = false
      ws = null
      if (!ended(task.value.status)) {
        setTimeout(() => { if (!ws && !ended(task.value.status)) startWS() }, 2000)
      }
    }
    ws.onerror = () => { wsConnected.value = false }
    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        if (msg.type === 'log') {
          appendLogs([{ seq: msg.seq, stream: msg.stream, content: msg.content }])
        } else if (msg.type === 'status') {
          if (msg.status) task.value.status = msg.status
          if (msg.stage) task.value.stage = msg.stage
        } else if (msg.type === 'done') {
          setTimeout(() => loadAll({ silent: true }), 400)
        }
      } catch { /* ignore */ }
    }
  } catch { wsConnected.value = false }
}

function stopWS() {
  if (ws) { try { ws.close() } catch { /* ignore */ } ws = null }
  wsConnected.value = false
}

// 兜底轮询：WS 未连接时定时拉取增量日志
function startPolling() {
  if (timer) return
  timer = setInterval(async () => {
    if (ended(task.value.status)) {
      stopPolling()
      return
    }
    try {
      const r = await getTask(id.value)
      task.value = r.data || task.value
      if (!wsConnected.value) {
        const lr = await taskLogs(id.value, { after_seq: lastSeq, limit: 1000 })
        appendLogs(Array.isArray(lr.data) ? lr.data : [])
      }
      if (ended(task.value.status)) {
        const pr = await taskPatch(id.value)
        patchText.value = pr.data?.patch || ''
        stopPolling()
        stopWS()
      }
    } catch { /* ignore */ }
  }, 3000)
}

function stopPolling() { if (timer) { clearInterval(timer); timer = null } }

function onTermScroll() {
  const el = termRef.value
  if (!el) return
  const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 48
  if (autoScroll.value !== nearBottom) autoScroll.value = nearBottom
}

function scrollBottom() {
  if (!autoScroll.value) return
  nextTick(() => {
    if (termRef.value) termRef.value.scrollTop = termRef.value.scrollHeight
  })
}

function copyLogs() {
  copyText(logs.value.map(l => l.content).join('\n'))
}
function copyPatch() { copyText(patchText.value) }

async function confirmFix() {
  const { value } = await ElMessageBox.prompt('确认后将提交代码、推送到远端并执行发布钩子', '人工确认', {
    inputPlaceholder: '备注（可选）', inputValue: ''
  }).catch(() => ({ value: null }))
  if (value === null) return
  await confirmTask(id.value, { operator: 'web', note: value })
  ElMessage.success('已确认，正在提交推送')
  setTimeout(() => loadAll({ silent: true }), 1500)
}

async function rejectFix() {
  const { value } = await ElMessageBox.prompt('填写驳回原因', '驳回修复', {
    inputPlaceholder: '如：定位不准，需人工介入', inputValue: ''
  }).catch(() => ({ value: null }))
  if (value === null) return
  await rejectTask(id.value, { operator: 'web', note: value })
  ElMessage.success('已驳回')
  loadAll({ silent: true })
}

async function retryTaskDo() {
  const r = await retryTask(id.value)
  ElMessage.success('已创建重试任务 #' + r.data.id)
  if (r.data?.id) {
    await router.push('/tasks/' + r.data.id)
    return
  }
  loadAll({ silent: true })
}

async function cancelTaskDo() {
  await cancelTask(id.value)
  ElMessage.success('已取消')
  loadAll({ silent: true })
}

watch(autoScroll, (v) => { if (v) scrollBottom() })
watch(id, (next, prev) => {
  if (String(next) === String(prev)) return
  stopPolling()
  stopWS()
  task.value = {}
  logs.value = []
  logIndex = new Set()
  lastSeq = 0
  patchText.value = ''
  loadAll()
})

onMounted(() => { loadAll() })
onBeforeUnmount(() => { stopPolling(); stopWS() })
</script>

<style scoped>
.am-task-detail {
  height: 100%;
  min-height: 0;
  max-width: 100%;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding-bottom: 12px;
}
.am-task-detail > .am-toolbar {
  flex: none;
}
.task-body {
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  gap: 16px;
  overflow: hidden;
}
.task-process {
  flex: 1 1 0;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.term-card {
  flex: 1;
  min-height: 0;
  min-width: 0;
  max-width: 100%;
  margin-bottom: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.term-toolbar {
  flex: none;
  margin-bottom: 10px;
}
.term-title {
  font-weight: 600;
}
.term-meta {
  font-size: 12px;
}
.term-card :deep(.am-terminal) {
  flex: 1;
  min-height: 0;
  height: auto;
}
.task-side {
  flex: 0 0 360px;
  width: 360px;
  min-width: 0;
  min-height: 0;
  overflow-x: hidden;
  overflow-y: auto;
}
.am-card {
  max-width: 100%;
  min-width: 0;
  overflow: hidden;
}
.am-task-detail :deep(.el-descriptions__content),
.am-task-detail :deep(.el-descriptions__label) {
  overflow-wrap: anywhere;
  word-break: break-word;
}
@media (max-width: 1024px) {
  .am-task-detail {
    height: auto;
    overflow: auto;
  }
  .task-body {
    flex-direction: column;
    overflow: visible;
  }
  .task-process {
    min-height: 420px;
  }
  .task-side {
    flex: none;
    width: 100%;
    overflow: visible;
  }
}
</style>
