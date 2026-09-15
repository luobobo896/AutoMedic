<template>
  <div class="am-page">
    <div class="am-page-head">
      <div>
        <div class="am-page-head__crumb">首页</div>
        <h1 class="am-page-head__title">事件中心</h1>
        <p class="am-page-head__desc">
          告警入库的第一站。同一指纹的事件自动合并成一条；命中规则才开修复流程，非代码问题直接忽略。
        </p>
      </div>
      <div class="am-page-head__actions">
        <el-button :icon="'Refresh'" aria-label="刷新" @click="load" />
      </div>
    </div>

    <div class="am-card">
      <div class="am-toolbar">
        <el-select v-model="query.level" clearable placeholder="级别" style="width: 120px" @change="search">
          <el-option label="FATAL" value="fatal" />
          <el-option label="ERROR" value="error" />
          <el-option label="WARN" value="warn" />
          <el-option label="INFO" value="info" />
        </el-select>
        <el-select v-model="query.status" clearable placeholder="状态" style="width: 140px" @change="search">
          <el-option v-for="(v, k) in EVENT_STATUS_META" :key="k" :label="v.label" :value="k" />
        </el-select>
        <el-select v-model="query.project_id" clearable placeholder="全部应用" style="width: 170px" @change="search">
          <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
        <el-select v-model="query.days" style="width: 130px" @change="search">
          <el-option label="近 24 小时" :value="1" />
          <el-option label="近 7 天" :value="7" />
          <el-option label="近 30 天" :value="30" />
          <el-option label="全部时间" :value="0" />
        </el-select>
        <el-input
          v-model="query.keyword"
          placeholder="事件号 / 标题 / 日志关键字"
          clearable
          style="width: 220px"
          @keyup.enter="search"
          @clear="search"
        />
        <el-button type="primary" :icon="'Search'" @click="search">查询</el-button>
      </div>

      <el-table :data="list" v-loading="loading" size="small">
        <el-table-column label="事件号" width="176">
          <template #default="{ row }">
            <button type="button" class="am-link am-mono" @click="openDetail(row)">{{ eventCode(row) }}</button>
          </template>
        </el-table-column>
        <el-table-column label="级别" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="LEVEL_META[row.level]?.type">{{ LEVEL_META[row.level]?.label || row.level }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="标题" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <button type="button" class="am-link" @click="openDetail(row)">{{ row.title }}</button>
          </template>
        </el-table-column>
        <el-table-column label="应用" width="140">
          <template #default="{ row }">{{ row.project?.name || '-' }}</template>
        </el-table-column>
        <el-table-column label="状态" width="140">
          <template #default="{ row }">
            <el-tag size="small" :type="EVENT_STATUS_META[row.status]?.type">
              {{ EVENT_STATUS_META[row.status]?.label || row.status }}
            </el-tag>
            <div v-if="row.dispose_msg" class="am-text-faint am-hint">{{ row.dispose_msg }}</div>
          </template>
        </el-table-column>
        <el-table-column label="重复" width="64" align="right">
          <template #default="{ row }">{{ row.occurrence_n || 1 }}</template>
        </el-table-column>
        <el-table-column label="来源" width="96">
          <template #default="{ row }">{{ row.source || '-' }}</template>
        </el-table-column>
        <el-table-column label="发生时间" width="140">
          <template #default="{ row }">{{ formatTimeShort(row.occurred_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="116">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDetail(row)">详情</el-button>
            <el-button
              link
              type="primary"
              :disabled="!canReplay(row)"
              :loading="replayingId === row.id"
              :title="replayHint(row)"
              :aria-disabled="!canReplay(row)"
              @click="replay(row)"
            >重放</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <div class="am-empty">
            <div class="am-empty__title">还没有事件进入</div>
            <div class="am-empty__desc">
              在项目里生成接入令牌，把告警源按 X-AM-Token 投递到 <span class="am-mono">/api/v1/ingest/events</span> 即可。
            </div>
            <div class="am-empty__actions">
              <el-button size="small" @click="$router.push('/tokens')">生成接入令牌</el-button>
              <el-button size="small" @click="$router.push('/rules')">配置修复规则</el-button>
            </div>
          </div>
        </template>
      </el-table>

      <el-pagination class="am-pagination"
        layout="total, sizes, prev, pager, next" :total="total"
        v-model:current-page="query.page" v-model:page-size="query.page_size"
        @current-change="load" @size-change="search" />
    </div>

    <el-drawer v-model="drawer" :size="isMobile ? '100%' : '72%'" class="am-detail-drawer">
      <template #header>
        <div class="ev-head">
          <el-tag v-if="detail.event" size="small" :type="LEVEL_META[detail.event.level]?.type">
            {{ LEVEL_META[detail.event.level]?.label || detail.event.level }}
          </el-tag>
          <el-tag v-if="detail.event" size="small" :type="EVENT_STATUS_META[detail.event.status]?.type">
            {{ EVENT_STATUS_META[detail.event.status]?.label }}
          </el-tag>
          <span class="am-mono ev-head__id">{{ eventCode(detail.event) }}</span>
          <span class="ev-head__title">{{ detail.event?.title }}</span>
          <el-tag v-if="(detail.event?.occurrence_n || 1) > 1" size="small" effect="plain">
            重复 {{ detail.event.occurrence_n }} 次
          </el-tag>
        </div>
      </template>

      <template v-if="detail.event">
        <div class="am-detail-grid">
          <div>
            <div class="am-card am-card--flush">
              <div class="am-meta-grid">
                <div>
                  <div class="am-meta__label">事件号</div>
                  <div class="am-meta__value am-mono">{{ eventCode(detail.event) }}</div>
                </div>
                <div>
                  <div class="am-meta__label">归属应用</div>
                  <div class="am-meta__value">{{ detail.event.project?.name || '-' }}</div>
                </div>
                <div>
                  <div class="am-meta__label">来源</div>
                  <div class="am-meta__value">{{ detail.event.source || '-' }}</div>
                </div>
                <div>
                  <div class="am-meta__label">指纹</div>
                  <div class="am-meta__value am-mono">{{ detail.event.fingerprint || '-' }}</div>
                </div>
                <div>
                  <div class="am-meta__label">命中规则</div>
                  <div class="am-meta__value">{{ detail.event.rule?.name || '未命中' }}</div>
                </div>
                <div>
                  <div class="am-meta__label">首次发生</div>
                  <div class="am-meta__value">{{ formatTime(detail.event.occurred_at) }}</div>
                </div>
                <div>
                  <div class="am-meta__label">最近发生</div>
                  <div class="am-meta__value">{{ formatTime(detail.event.updated_at || detail.event.occurred_at) }}</div>
                </div>
              </div>
              <div v-if="detail.event.dispose_msg" class="am-section ev-dispose">
                <span class="am-meta__label">处置结论</span>
                <div>{{ detail.event.dispose_msg }}</div>
              </div>
            </div>

            <div class="am-card am-card--flush ev-log">
              <el-tabs v-model="logTab">
                <el-tab-pane label="归一化日志" name="stack">
                  <div class="am-diff">{{ detail.event.stack || detail.event.message || '-' }}</div>
                </el-tab-pane>
                <el-tab-pane label="原始 Payload" name="payload">
                  <div class="am-diff">{{ prettyPayload }}</div>
                </el-tab-pane>
              </el-tabs>
            </div>
          </div>

          <aside class="am-card am-card--flush">
            <div class="am-toolbar"><span class="am-card__title">处置链路</span></div>
            <ol class="am-timeline">
              <li v-for="s in timeline" :key="s.title" class="am-timeline__item" :class="s.cls">
                <div class="am-timeline__title">
                  {{ s.title }}<span class="am-timeline__state">{{ s.state }}</span>
                </div>
                <div v-if="s.desc" class="am-timeline__desc">{{ s.desc }}</div>
              </li>
            </ol>

            <template v-if="(detail.tasks || []).length">
              <div class="am-toolbar" style="margin-top: 16px"><span class="am-card__title">关联流程</span></div>
              <div class="ev-task" v-for="t in detail.tasks" :key="t.id">
                <div class="ev-task__main">
                  <span class="am-mono">#{{ t.id }}</span>
                  <el-tag size="small" :type="STATUS_META[t.status]?.type">{{ STATUS_META[t.status]?.label }}</el-tag>
                  <span class="am-text-dim am-hint">{{ t.repo?.name || '-' }}</span>
                </div>
                <el-button link type="primary" @click="gotoTask(t)">进入监控</el-button>
              </div>
            </template>
          </aside>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listEvents, getEvent, replayEvent, listProjects } from '@/api'
import { LEVEL_META, EVENT_STATUS_META, STATUS_META, formatTime, formatTimeShort, parseJSON, eventCode } from '@/utils/format'
import { useBreakpoint } from '@/composables/useBreakpoint'
import { ElMessage, ElMessageBox } from 'element-plus'

const router = useRouter()
const route = useRoute()
const { isMobile } = useBreakpoint()
const list = ref([])
const projects = ref([])
const total = ref(0)
const loading = ref(false)
const drawer = ref(false)
const logTab = ref('stack')
const detail = ref({})
const replayingId = ref(0)
const query = reactive({ page: 1, page_size: 20, project_id: '', status: '', level: '', keyword: '', days: 0 })

const prettyPayload = computed(() => JSON.stringify(parseJSON(detail.value.event?.payload, {}), null, 2))

// 处置链路：把「事件 → 规则 → 流程 → 确认 → 提交」的关键节点拼成一条可读链路，
// 每一步都取自真实字段（不推断、不编造），让运维一眼看出卡在哪一环。
const timeline = computed(() => {
  const e = detail.value.event || {}
  const tasks = detail.value.tasks || []
  const task = tasks[0]
  const steps = [{
    title: '事件接收',
    state: formatTime(e.occurred_at),
    cls: 'is-done',
    desc: `${e.source || '未知来源'} 投递，指纹 ${(e.fingerprint || '').slice(0, 12)}`
  }]

  const matched = !!e.rule
  steps.push({
    title: '规则判定',
    state: matched ? '已命中' : '未命中',
    cls: matched ? 'is-done' : 'is-failed',
    desc: matched ? `命中规则「${e.rule.name}」` : (e.dispose_msg || '未命中任何规则，未开流程')
  })

  if (!task) {
    steps.push({
      title: '修复流程',
      state: '未创建',
      cls: '',
      desc: e.status === 'dropped' || e.status === 'ignored' ? '按规则丢弃/忽略，不进入自愈' : '等待规则命中后创建'
    })
    return steps
  }

  const running = ['pending', 'running', 'confirming'].includes(task.status)
  steps.push({
    title: '修复流程',
    state: `#${task.id} ${STATUS_META[task.status]?.label || task.status}`,
    cls: running ? 'is-active' : task.status === 'success' ? 'is-done' : 'is-failed',
    desc: `${task.project?.name || '-'} · ${task.repo?.name || '-'} · ${task.mode === 'auto' ? '全自动' : '半自动'}`,
    })

  steps.push({
    title: '根因分析与修复',
    state: task.diagnosis ? '已产出' : running ? '进行中' : '无产出',
    cls: task.diagnosis ? 'is-done' : running ? 'is-active' : '',
    desc: task.diagnosis || '等待 dsh 输出根因'
  })

  if (task.mode !== 'auto') {
    steps.push({
      title: '人工确认',
      state: task.confirmed_by ? `由 ${task.confirmed_by} 确认` : task.status === 'confirming' ? '待确认' : '未确认',
      cls: task.confirmed_by ? 'is-done' : task.status === 'confirming' ? 'is-active' : '',
      desc: task.confirmed_at ? formatTime(task.confirmed_at) : '半自动流程需要人工确认后才推送'
    })
  }

  steps.push({
    title: '提交与推送',
    state: task.fix_commit ? '已提交' : task.status === 'failed' ? '未完成' : '待执行',
    cls: task.fix_commit ? 'is-done' : task.status === 'failed' ? 'is-failed' : '',
    desc: task.fix_commit ? `${task.branch || '-'} @ ${(task.fix_commit || '').slice(0, 10)}` : (task.error_msg || '')
  })

  return steps
})

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
  logTab.value = 'stack'
  const r = await getEvent(row.id)
  detail.value = r.data || {}
  drawer.value = true
}

function gotoTask(t) {
  drawer.value = false
  router.push('/tasks/' + t.id)
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
  if (!canReplay(row) || replayingId.value) return
  replayingId.value = row.id
  try {
    const ok = await ElMessageBox.confirm('重放会按同一指纹重新走规则。已修复或修复中的事件不会再开任务。', '提示', { type: 'warning' }).catch(() => false)
    if (!ok) return
    const r = await replayEvent(row.id)
    ElMessage.success(r.data?.reason || '已重放')
    await load()
  } finally { replayingId.value = 0 }
}

onMounted(async () => {
  // 流程监控通过 ?code=INC-… 跳进来：直接把业务号灌进关键字，省掉手工粘贴
  if (route.query.code) query.keyword = String(route.query.code)
  const p = await listProjects({ page_size: 100 })
  projects.value = p.data?.list || []
  load()
})
</script>

<style scoped>
.ev-head { display: flex; align-items: center; gap: var(--am-space-2); flex-wrap: wrap; min-width: 0; }
.ev-head__id { color: var(--am-text-faint); }
.ev-head__title { font-size: var(--am-font-lg); font-weight: 600; color: var(--am-text); overflow-wrap: anywhere; }
.ev-dispose { margin-top: var(--am-space-3); font-size: var(--am-font-sm); }
.ev-log { margin-top: var(--am-space-4); }
.ev-task {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--am-space-2);
  padding: 10px 0;
  border-top: 1px solid var(--am-border-subtle);
}
.ev-task__main { display: flex; align-items: center; gap: 8px; min-width: 0; flex-wrap: wrap; }
</style>
