<template>
  <div class="am-page">
    <!-- 核心四卡：一眼看清任务盘面；次要指标降级为摘要行 -->
    <div class="am-stat-grid">
      <div class="am-stat">
        <div class="label">修复任务</div>
        <div class="value">{{ t.total || 0 }}</div>
      </div>
      <div class="am-stat">
        <div class="label">待人工确认</div>
        <div class="value am-value--warning">{{ t.confirming || 0 }}</div>
      </div>
      <div class="am-stat">
        <div class="label">修复成功</div>
        <div class="value am-value--success">{{ t.success || 0 }}</div>
      </div>
      <div class="am-stat">
        <div class="label">失败</div>
        <div class="value am-value--danger">{{ t.failed || 0 }}</div>
      </div>
    </div>
    <div class="am-substats">
      项目 {{ ov.projects || 0 }} · 仓库 {{ ov.repos || 0 }} · 累计事件 {{ ov.events || 0 }}
      · 已忽略 {{ t.ignored || 0 }} · 今日任务 {{ ov.today?.total || 0 }} / 成功 {{ ov.today?.success || 0 }}
    </div>

    <el-row :gutter="16">
      <el-col :xs="24" :md="14">
        <div class="am-card">
          <div class="am-toolbar">
            <span class="am-card__title">任务趋势（近 14 天）</span>
            <div class="am-flex-1" />
            <el-button size="small" @click="$router.push('/statistics')">查看统计</el-button>
          </div>
          <div ref="trendRef" style="height:300px" />
        </div>
      </el-col>
      <el-col :xs="24" :md="10">
        <div class="am-card">
          <div class="am-toolbar"><span class="am-card__title">任务状态分布</span></div>
          <div ref="pieRef" style="height:300px" />
        </div>
      </el-col>
    </el-row>

    <div class="am-card">
      <div class="am-toolbar">
        <span class="am-card__title">最近的修复任务</span>
        <div class="am-flex-1" />
        <el-button size="small" type="primary" @click="$router.push('/tasks')">全部任务</el-button>
      </div>
      <el-table :data="tasks" size="small" @row-click="row => $router.push('/tasks/' + row.id)">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="STATUS_META[row.status]?.type">{{ STATUS_META[row.status]?.label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="项目 / 仓库">
          <template #default="{ row }">
            {{ row.project?.name || '-' }} · {{ row.repo?.name || '-' }}
          </template>
        </el-table-column>
        <el-table-column label="触发事件" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">{{ row.event?.title || '-' }}</template>
        </el-table-column>
        <el-table-column label="模式" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.mode === 'auto' ? 'danger' : 'warning'" effect="plain">
              {{ row.mode === 'auto' ? '全自动' : '半自动' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="耗时" width="90">
          <template #default="{ row }">{{ formatDuration(row.duration_ms) }}</template>
        </el-table-column>
        <el-table-column label="创建时间" width="160">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { overview, listTasks, statsTrend } from '@/api'
import { STATUS_META, formatDuration, formatTime } from '@/utils/format'
import { AM_COLORS } from '@/constants/tokens'
import * as echarts from 'echarts'

const ov = ref({ tasks: {}, today: {} })
const tasks = ref([])
const trendRef = ref()
const pieRef = ref()
let trendChart, pieChart

const t = computed(() => ov.value.tasks || {})

async function load() {
  const [o, tk, tr] = await Promise.all([
    overview(),
    listTasks({ page: 1, page_size: 10 }),
    statsTrend({ days: 14 })
  ])
  ov.value = o.data || o
  tasks.value = tk.data?.list || []
  renderTrend(tr.data || [])
  renderPie()
}

function renderTrend(rows) {
  if (!trendRef.value) return
  trendChart = trendChart || echarts.init(trendRef.value)
  trendChart.setOption({
    backgroundColor: 'transparent',
    tooltip: { trigger: 'axis' },
    legend: { data: ['成功', '失败', '已忽略', '处理中'], textStyle: { color: AM_COLORS.labelText }, top: 0 },
    grid: { left: 40, right: 20, top: 40, bottom: 30 },
    xAxis: { type: 'category', data: rows.map(r => r.day?.slice(5)), axisLine: { lineStyle: { color: AM_COLORS.axisLine } }, axisLabel: { color: AM_COLORS.labelText } },
    yAxis: { type: 'value', splitLine: { lineStyle: { color: AM_COLORS.splitLine } }, axisLabel: { color: AM_COLORS.labelText } },
    series: [
      { name: '成功', type: 'bar', stack: 'a', data: rows.map(r => r.success), itemStyle: { color: AM_COLORS.success } },
      { name: '失败', type: 'bar', stack: 'a', data: rows.map(r => r.failed), itemStyle: { color: AM_COLORS.danger } },
      { name: '已忽略', type: 'bar', stack: 'a', data: rows.map(r => r.ignored), itemStyle: { color: AM_COLORS.neutral } },
      { name: '处理中', type: 'bar', stack: 'a', data: rows.map(r => r.pending), itemStyle: { color: AM_COLORS.primary } }
    ]
  }, true)
}

function renderPie() {
  if (!pieRef.value) return
  pieChart = pieChart || echarts.init(pieRef.value)
  const items = Object.keys(STATUS_META).map(k => ({
    name: STATUS_META[k].label,
    value: ov.value.tasks?.[k] || 0
  })).filter(i => i.value > 0)
  pieChart.setOption({
    backgroundColor: 'transparent',
    tooltip: { trigger: 'item' },
    legend: { bottom: 0, textStyle: { color: AM_COLORS.labelText } },
    series: [{
      type: 'pie', radius: ['45%', '68%'], center: ['50%', '45%'], data: items,
      label: { color: AM_COLORS.labelText },
      color: [AM_COLORS.primary, AM_COLORS.success, AM_COLORS.warning, AM_COLORS.danger, AM_COLORS.neutral]
    }]
  }, true)
}

function onResize() {
  trendChart?.resize()
  pieChart?.resize()
}

onMounted(() => {
  load()
  window.addEventListener('resize', onResize)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  trendChart?.dispose()
  pieChart?.dispose()
})
</script>
