<template>
  <div class="am-page am-task-detail" v-loading="booting">
    <div class="am-page-head">
      <div>
        <div class="am-page-head__crumb">
          <el-button link type="primary" @click="$router.push('/tasks')">修复流程</el-button> / 流程监控
        </div>
        <h1 class="am-page-head__title">
          流程 #{{ id }}
        </h1>
        <div class="am-page-head__desc">
          <el-tag :type="STATUS_META[task.status]?.type">{{ STATUS_META[task.status]?.label }}</el-tag>
          <el-tag v-if="showStageTag" type="info" effect="plain">{{ STAGE_LABEL[task.stage] || task.stage }}</el-tag>
          <el-tag effect="plain" :type="task.mode === 'auto' ? 'danger' : 'warning'">
            {{ task.mode === 'auto' ? '全自动' : '半自动确认' }}
          </el-tag>
          <router-link
            v-if="task.event"
            class="am-link am-mono"
            :to="{ path: '/events', query: { code: eventCode(task.event) } }"
          >
            {{ eventCode(task.event) }}
          </router-link>
          <span class="am-text-dim">
            {{ task.project?.name || '-' }} · {{ task.repo?.name || '-' }} · 耗时 {{ formatDuration(task.duration_ms) }}
          </span>
        </div>
      </div>
      <div class="am-page-head__actions">
        <el-button v-if="task.status === 'confirming'" type="success" :icon="'Check'"
          :loading="acting === 'confirm'" :disabled="!!acting" @click="confirmFix">确认修复并推送</el-button>
        <el-button v-if="task.status === 'confirming'" type="danger" :icon="'Close'"
          :disabled="!!acting" @click="rejectFix">驳回</el-button>
        <el-button
          v-if="showRetry"
          :type="canResumePush ? 'success' : 'primary'"
          :icon="canResumePush ? 'Upload' : 'RefreshRight'"
          :disabled="!canRetry || !!acting"
          :loading="acting === 'retry'"
          :aria-disabled="!canRetry || !!acting"
          :title="canRetry ? '' : (task.status === 'success' ? '任务已成功，无需重试' : '当前状态不能重试')"
          @click="canResumePush ? retryPushDo() : retryTaskDo()"
        >{{ canResumePush ? '重试推送' : '重试修复' }}</el-button>
        <el-button v-if="['pending','running'].includes(task.status)" :icon="'CircleClose'"
          :loading="acting === 'cancel'" :disabled="!!acting" @click="cancelTaskDo">取消</el-button>
        <el-button :icon="'Refresh'" aria-label="刷新" @click="loadAll" />
      </div>
    </div>

    <div class="am-card">
      <div class="am-toolbar">
        <span class="am-card__title">流程阶段</span>
        <span class="am-text-dim am-hint">每个阶段的产出都能在下方日志、补丁与结果里逐条核对</span>
      </div>
      <div class="am-stage-rail">
        <div v-for="(s, i) in stages" :key="s.title" class="am-stage" :class="s.cls">
          <div class="am-stage__head">
            <span class="am-stage__no">{{ i + 1 }}</span>
            <span class="am-stage__title">{{ s.title }}</span>
          </div>
          <div class="am-stage__rows">
            <div v-for="r in s.rows" :key="r.text" class="am-stage__row" :class="r.cls">
              <span class="am-stage__mark">{{ r.mark }}</span>
              <span class="am-stage__text">{{ r.text }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="task-body">
      <section class="task-process" aria-label="修复过程">
        <div class="am-card term-card">
          <el-tabs v-model="pane">
            <el-tab-pane label="过程日志" name="logs">
              <div class="am-toolbar term-toolbar">
                <el-tag v-if="wsConnected" size="small" type="success" effect="dark">实时连接</el-tag>
                <el-tag v-else size="small" type="info" effect="plain">离线（轮询）</el-tag>
                <span class="am-text-dim am-hint">{{ logs.length }} 行</span>
                <div class="am-flex-1" />
                <el-checkbox v-model="autoScroll" size="small">跟随输出</el-checkbox>
                <el-button size="small" @click="copyLogs">复制日志</el-button>
              </div>
              <div ref="termRef" class="am-terminal" @scroll="onTermScroll">
                <div v-for="(l, i) in logs" :key="l.seq || i" :class="'line-' + l.stream">
                  <span class="am-text-faint">[{{ l.seq }}]</span> {{ stripANSI(l.content) }}
                </div>
                <div v-if="!logs.length" class="am-text-faint">等待 dsh 输出…</div>
              </div>
            </el-tab-pane>
            <el-tab-pane :label="`补丁 Diff${patchLines.length > 1 ? '（' + (patchLines.length - 1) + ' 行）' : ''}`" name="patch">
              <div class="am-toolbar term-toolbar">
                <span class="am-text-dim am-hint">{{ task.diff_stat || '暂无 diff 统计' }}</span>
                <div class="am-flex-1" />
                <el-button size="small" @click="copyPatch" :disabled="!patchText">复制补丁</el-button>
              </div>
              <div class="am-diff am-diff--tall">
                <div v-for="(l, i) in patchLines" :key="i" :class="diffClass(l)">{{ l }}</div>
                <div v-if="!patchLines.length" class="am-text-faint">暂无补丁</div>
              </div>
            </el-tab-pane>
          </el-tabs>
        </div>

        <div class="am-card">
          <div class="am-toolbar"><span class="am-card__title">修复结果</span></div>
          <div class="am-meta-grid">
            <div class="am-meta--full">
              <div class="am-meta__label">根因分析</div>
              <div class="am-meta__value">{{ task.diagnosis || '-' }}</div>
            </div>
            <div class="am-meta--full">
              <div class="am-meta__label">修复摘要</div>
              <div class="am-meta__value am-meta__value--pre">{{ task.summary || '-' }}</div>
            </div>
          </div>
          <div class="am-meta-grid am-meta-grid--2 am-mt-4">
            <div>
              <div class="am-meta__label">变更文件</div>
              <div v-if="changedFiles.length" class="am-filelist">
                <div v-for="f in changedFiles" :key="f" class="am-filelist__item">{{ f }}</div>
              </div>
              <div v-else class="am-meta__value am-text-faint">-</div>
            </div>
            <div>
              <div class="am-meta__label">变更统计</div>
              <div class="am-meta__value am-mono">{{ task.diff_stat || '-' }}</div>
              <div class="am-meta__label am-mt-3">错误信息</div>
              <div class="am-meta__value" :class="{ 'am-text-danger': task.error_msg }">{{ task.error_msg || '-' }}</div>
            </div>
          </div>
        </div>
      </section>

      <aside class="task-side" aria-label="任务信息与结果">
        <div class="am-card">
          <div class="am-toolbar"><span class="am-card__title">任务信息</span></div>
          <div class="am-meta-grid">
            <div>
              <div class="am-meta__label">项目 / 仓库</div>
              <div class="am-meta__value">{{ task.project?.name || '-' }} · {{ task.repo?.name || '-' }}</div>
            </div>
            <div>
              <div class="am-meta__label">分支</div>
              <div class="am-meta__value am-mono">{{ task.branch || '-' }}</div>
            </div>
            <div>
              <div class="am-meta__label">基线提交</div>
              <div class="am-meta__value am-mono">{{ (task.base_commit || '-').slice(0, 12) }}</div>
            </div>
            <div>
              <div class="am-meta__label">修复提交</div>
              <div class="am-meta__value am-mono">{{ (task.fix_commit || '-').slice(0, 12) }}</div>
            </div>
            <div>
              <div class="am-meta__label">模型</div>
              <div class="am-meta__value">
                {{ task.model?.name || task.dsh_model || '-' }}
                <div class="am-text-faint am-hint">
                  provider={{ task.dsh_provider || '-' }} ctx={{ formatTokens(task.input_context) }}/{{ formatTokens(task.output_context) }}
                </div>
              </div>
            </div>
            <div>
              <div class="am-meta__label">命中规则</div>
              <div class="am-meta__value">{{ task.rule?.name || '-' }}</div>
            </div>
            <div>
              <div class="am-meta__label">重试次数</div>
              <div class="am-meta__value">{{ task.retry || 0 }}</div>
            </div>
            <div>
              <div class="am-meta__label">开始 / 结束</div>
              <div class="am-meta__value">{{ formatTime(task.started_at) }} → {{ formatTime(task.finished_at) }}</div>
            </div>
            <div>
              <div class="am-meta__label">耗时</div>
              <div class="am-meta__value">{{ formatDuration(task.duration_ms) }}</div>
            </div>
            <div>
              <div class="am-meta__label">工作区</div>
              <div class="am-meta__value am-mono">{{ task.workspace || '-' }}</div>
            </div>
          </div>
        </div>

        <div class="am-card">
          <div class="am-toolbar"><span class="am-card__title">人工确认</span></div>
          <template v-if="task.mode === 'auto'">
            <div class="am-text-dim am-hint">全自动模式：修复完成后直接提交推送，无需人工确认。</div>
          </template>
          <el-descriptions v-else :column="1" border size="small">
            <el-descriptions-item label="确认状态">
              <el-tag v-if="task.confirmed_by" size="small" type="success">已确认</el-tag>
              <el-tag v-else-if="task.status === 'confirming'" size="small" type="warning">待确认</el-tag>
              <el-tag v-else-if="task.status === 'rejected'" size="small" type="danger">已驳回</el-tag>
              <span v-else class="am-text-faint">未到确认环节</span>
            </el-descriptions-item>
            <el-descriptions-item label="确认人">
              {{ task.confirmed_by || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="确认时间">{{ formatTime(task.confirmed_at) }}</el-descriptions-item>
            <el-descriptions-item label="确认备注">{{ task.confirm_note || '-' }}</el-descriptions-item>
          </el-descriptions>
        </div>

        <div class="am-card">
          <div class="am-toolbar">
            <span class="am-card__title">触发事件</span>
            <div class="am-flex-1" />
            <router-link
              v-if="task.event"
              class="am-link"
              :to="{ path: '/events', query: { code: eventCode(task.event) } }"
            >在事件中心打开</router-link>
          </div>
          <template v-if="task.event">
            <div style="margin-bottom:6px">
              <el-tag size="small" :type="LEVEL_META[task.event.level]?.type">{{ task.event.level }}</el-tag>
              <span style="margin-left:6px">{{ task.event.title }}</span>
            </div>
            <div class="am-text-dim" style="font-size: var(--am-font-xs);margin-bottom:6px">
              {{ eventCode(task.event) }} · {{ task.event.source }} · {{ formatTime(task.event.occurred_at) }}
            </div>
            <div class="am-diff" style="max-height:220px">{{ task.event.stack || task.event.message || '-' }}</div>
          </template>
          <span v-else class="am-text-dim">手动触发</span>
        </div>

        <div class="am-card">
          <div class="am-toolbar"><span class="am-card__title">dsh 调用命令</span></div>
          <div class="am-diff" style="max-height:140px; font-size: var(--am-font-xs)">{{ task.dsh_cmd || '-' }}</div>
          <div class="am-text-dim" style="font-size: var(--am-font-xs);margin-top:6px">
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
import { getTask, taskLogs, taskPatch, confirmTask, rejectTask, retryTask, cancelTask, createTaskWS } from '@/api'
import { STATUS_META, STAGE_LABEL, LEVEL_META, formatTime, formatDuration, formatTokens, stripANSI, parseJSON, copyText, eventCode } from '@/utils/format'
import { ElMessage, ElMessageBox } from 'element-plus'

const route = useRoute()
const router = useRouter()
const id = computed(() => route.params.id)
const task = ref({})
const logs = ref([])
const patchText = ref('')
const pane = ref('logs')
const booting = ref(true)
const autoScroll = ref(true)
const wsConnected = ref(false)
// 写操作 in-flight 守卫：'' | 'confirm' | 'reject' | 'retry' | 'cancel'
const acting = ref('')
const termRef = ref()

let ws = null
let timer = null
let wsRetryTimer = null
let wsBackoff = 2000
let unmounted = false
let lastSeq = 0
let logIndex = new Set()
let loadGen = 0

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
// 状态标签与阶段标签同义时不重复展示（如 status=confirming 且 stage=confirming）
const showStageTag = computed(() =>
  !!task.value.stage && STAGE_LABEL[task.value.stage] !== STATUS_META[task.value.status]?.label)

// 流程阶段轨道：把后端真实字段映射成 7 步流水线（不新增状态机，只做呈现聚合）。
// 每步的「完成/进行/失败/跳过」都能在右侧日志或下方结果里找到对应证据。
const stages = computed(() => {
  const k = task.value
  const running = ['pending', 'running', 'confirming'].includes(k.status)
  const failed = k.status === 'failed'
  const semi = k.mode !== 'auto'
  const step = (title, cls, rows) => ({ title, cls, rows })
  const mark = (ok, waitText, doneText) => ({ mark: ok ? '✓' : (running && !failed ? '…' : '–'), cls: ok ? 'is-ok' : 'is-wait', text: ok ? doneText : waitText })

  return [
    step('接入与解析', k.event_id ? 'is-done' : 'is-active', [
      mark(k.event_id, '手动触发，无来源事件', k.event?.title || '已接收事件'),
      mark(!!k.rule?.name, '未命中规则', `命中规则「${k.rule?.name}」`)
    ]),
    step('准备与根因分析', k.diagnosis ? 'is-done' : failed ? 'is-failed' : running ? 'is-active' : '', [
      mark(!!k.workspace, '等待分配隔离工作区', `工作区 ${String(k.workspace || '').split('/').pop()}`),
      mark(!!k.diagnosis, failed ? '未产出根因' : 'dsh 分析中', '已产出根因分析')
    ]),
    step('修复方案', changedFiles.value.length ? 'is-done' : failed ? 'is-failed' : running ? 'is-active' : '', [
      mark(changedFiles.value.length > 0, '暂无文件改动', `${changedFiles.value.length} 个文件变更`),
      mark(!!k.summary, '暂无修复摘要', '已产出修复摘要')
    ]),
    step('人工确认', semi ? (k.confirmed_by ? 'is-done' : k.status === 'confirming' ? 'is-active' : '') : 'is-done', [
      semi
        ? mark(!!k.confirmed_by, k.status === 'confirming' ? '等待人工确认' : '未确认', `由 ${k.confirmed_by} 确认`)
        : { mark: '–', cls: 'is-wait', text: '全自动模式，无需审批' },
      semi && k.confirmed_at
        ? { mark: '✓', cls: 'is-ok', text: formatTime(k.confirmed_at) }
        : { mark: '–', cls: 'is-wait', text: k.mode === 'auto' ? 'BR：全自动直推' : '确认后才会推送' }
    ]),
    step('修复执行', k.patch ? 'is-done' : failed ? 'is-failed' : running ? 'is-active' : '', [
      mark(!!k.patch, '暂无补丁产出', `补丁 ${patchLines.value.length - 1} 行`),
      { mark: k.dsh_exit_code === 0 ? '✓' : (k.dsh_exit_code ? '×' : '–'), cls: k.dsh_exit_code === 0 ? 'is-ok' : (k.dsh_exit_code ? 'is-failed' : 'is-wait'), text: `dsh 退出码 ${k.dsh_exit_code ?? '-'}` }
    ]),
    step('提交与推送', k.fix_commit ? 'is-done' : failed ? 'is-failed' : '', [
      mark(!!k.fix_commit, '未提交', `提交 ${(k.fix_commit || '').slice(0, 10)}`),
      mark(!!k.pr_url, k.branch ? `分支 ${k.branch}` : '无分支', k.pr_url)
    ]),
    step('流程结果', ['success', 'failed', 'ignored', 'rejected', 'cancelled'].includes(k.status) ? (k.status === 'success' ? 'is-done' : k.status === 'ignored' || k.status === 'cancelled' ? '' : 'is-failed') : 'is-active', [
      { mark: k.status === 'success' ? '✓' : failed ? '×' : '–', cls: k.status === 'success' ? 'is-ok' : failed ? 'is-failed' : 'is-wait', text: STATUS_META[k.status]?.label || '进行中' },
      { mark: '–', cls: 'is-wait', text: k.error_msg || `耗时 ${formatDuration(k.duration_ms)}` }
    ])
  ]
})
const canResumePush = computed(() => {
  if (task.value.status !== 'failed') return false
  return !!(task.value.patch || task.value.fix_commit || task.value.workspace)
})
const showRetry = computed(() => ['success', 'failed', 'ignored', 'rejected', 'cancelled'].includes(task.value.status))
const canRetry = computed(() => canResumePush.value || ['failed', 'ignored', 'rejected', 'cancelled'].includes(task.value.status))
const taskId = computed(() => {
  const n = Number(id.value)
  return Number.isInteger(n) && n > 0 ? n : 0
})

function diffClass(line) {
  if (line.startsWith('+') && !line.startsWith('+++')) return 'add'
  if (line.startsWith('-') && !line.startsWith('---')) return 'del'
  if (line.startsWith('@@')) return 'hunk'
  return ''
}

async function loadAll({ silent } = {}) {
  if (!taskId.value) {
    booting.value = false
    return
  }
  const gen = ++loadGen
  const tid = taskId.value
  if (!silent && !task.value.id) booting.value = true
  try {
    const r = await getTask(tid)
    if (gen !== loadGen) return
    task.value = r.data || {}
    const lr = await taskLogs(tid, silent ? { after_seq: lastSeq, limit: 1000 } : { limit: 2000 })
    if (gen !== loadGen) return
    if (!silent) {
      logs.value = []
      logIndex = new Set()
      lastSeq = 0
    }
    appendLogs(Array.isArray(lr.data) ? lr.data : [])
    if (ended(task.value.status) || task.value.patch || task.value.diff_stat) {
      const pr = await taskPatch(tid)
      if (gen !== loadGen) return
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
    if (gen === loadGen) booting.value = false
  }
}

function startWS() {
  if (ws || unmounted || ended(task.value.status) || !taskId.value) return
  try {
    ws = createTaskWS(taskId.value)
    if (!ws) { wsConnected.value = false; return }
    ws.onopen = () => { wsConnected.value = true; wsBackoff = 2000 }
    ws.onclose = () => {
      wsConnected.value = false
      ws = null
      if (!unmounted && !ended(task.value.status)) {
        clearTimeout(wsRetryTimer)
        // 指数退避 2s→4s→8s…上限 30s，避免服务端不可用时每 2s 打一次握手（轮询仍兜底）
        const delay = wsBackoff
        wsBackoff = Math.min(wsBackoff * 2, 30000)
        wsRetryTimer = setTimeout(() => {
          wsRetryTimer = null
          if (!ws && !unmounted && !ended(task.value.status)) startWS()
        }, delay)
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
  if (wsRetryTimer) { clearTimeout(wsRetryTimer); wsRetryTimer = null }
  if (ws) { try { ws.close() } catch { /* ignore */ } ws = null }
  wsConnected.value = false
}

// 兜底轮询：WS 未连接时定时拉取增量日志
function startPolling() {
  if (timer) return
  timer = setInterval(async () => {
    if (!taskId.value || ended(task.value.status)) {
      stopPolling()
      return
    }
    try {
      const r = await getTask(taskId.value)
      task.value = r.data || task.value
      if (!wsConnected.value) {
        const lr = await taskLogs(taskId.value, { after_seq: lastSeq, limit: 1000 })
        appendLogs(Array.isArray(lr.data) ? lr.data : [])
      }
      if (ended(task.value.status)) {
        const pr = await taskPatch(taskId.value)
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
  if (acting.value) return
  acting.value = 'confirm'
  try {
    const { value } = await ElMessageBox.prompt('确认后将提交代码、推送到远端并执行发布钩子', '人工确认', {
      inputPlaceholder: '备注（可选）', inputValue: ''
    }).catch(() => ({ value: null }))
    if (value === null) return
    await confirmTask(taskId.value, { operator: 'web', note: value })
    ElMessage.success('已确认，正在提交推送')
    // 守卫保持到状态刷新完成，避免刷新窗口内再次点击重复提交/重复推送
    await new Promise(done => setTimeout(done, 1500))
    await loadAll({ silent: true })
  } finally { acting.value = '' }
}

async function rejectFix() {
  if (acting.value) return
  acting.value = 'reject'
  try {
    const { value } = await ElMessageBox.prompt('填写驳回原因', '驳回修复', {
      inputPlaceholder: '如：定位不准，需人工介入', inputValue: ''
    }).catch(() => ({ value: null }))
    if (value === null) return
    await rejectTask(taskId.value, { operator: 'web', note: value })
    ElMessage.success('已驳回')
    await loadAll({ silent: true })
  } finally { acting.value = '' }
}

async function retryPushDo() {
  if (acting.value) return
  acting.value = 'retry'
  try {
    await retryTask(taskId.value)
    ElMessage.success('正在重试提交并推送，不会重新跑 dsh')
    await new Promise(done => setTimeout(done, 800))
    await loadAll({ silent: true })
  } finally { acting.value = '' }
}

async function retryTaskDo() {
  if (acting.value) return
  acting.value = 'retry'
  try {
    const r = await retryTask(taskId.value)
    if (r.data?.resume) {
      ElMessage.success('正在重试提交并推送，不会重新跑 dsh')
      await new Promise(done => setTimeout(done, 800))
      await loadAll({ silent: true })
      return
    }
    ElMessage.success('已创建重试任务 #' + r.data.id)
    if (r.data?.id) {
      await router.push('/tasks/' + r.data.id)
      return
    }
    await loadAll({ silent: true })
  } finally { acting.value = '' }
}

async function cancelTaskDo() {
  if (acting.value) return
  acting.value = 'cancel'
  try {
    await cancelTask(taskId.value)
    ElMessage.success('已取消')
    await loadAll({ silent: true })
  } finally { acting.value = '' }
}

watch(autoScroll, (v) => { if (v) scrollBottom() })
watch(id, (next, prev) => {
  if (String(next) === String(prev)) return
  loadGen++
  stopPolling()
  stopWS()
  task.value = {}
  logs.value = []
  logIndex = new Set()
  lastSeq = 0
  patchText.value = ''
  loadAll()
})

onMounted(() => { unmounted = false; loadAll() })
onBeforeUnmount(() => { unmounted = true; stopPolling(); stopWS() })
</script>

<style scoped>
.am-task-detail {
  height: 100%;
  min-height: 520px;
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
  flex: 1 1 0;
  min-height: 0;
  min-width: 0;
  display: flex;
  gap: 16px;
  overflow: hidden;
}
.task-process {
  flex: 1 1 0;
  min-width: 0;
  min-height: 320px;
  display: flex;
  flex-direction: column;
}
.term-card {
  flex: 1 1 0;
  min-height: 280px;
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
  font-size: var(--am-font-xs);
}
.term-card :deep(.am-terminal) {
  flex: 1 1 0;
  min-height: 200px;
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
@media (max-width: 1023.98px) {
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
