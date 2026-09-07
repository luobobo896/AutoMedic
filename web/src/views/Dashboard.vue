<template>
  <div class="am-page">
    <div class="am-stat-grid">
      <div class="am-stat">
        <div class="label">项目 / 仓库</div>
        <div class="value">{{ ov.projects || 0 }} / {{ ov.repos || 0 }}</div>
      </div>
      <div class="am-stat">
        <div class="label">累计事件</div>
        <div class="value">{{ ov.events || 0 }}</div>
      </div>
      <div class="am-stat">
        <div class="label">修复任务</div>
        <div class="value">{{ t.total || 0 }}</div>
      </div>
      <div class="am-stat">
        <div class="label">待人工确认</div>
        <div class="value" style="color:#f59f00">{{ t.confirming || 0 }}</div>
      </div>
      <div class="am-stat">
        <div class="label">修复成功</div>
        <div class="value" style="color:#37b24d">{{ t.success || 0 }}</div>
      </div>
      <div class="am-stat">
        <div class="label">失败</div>
        <div class="value" style="color:#f0506e">{{ t.failed || 0 }}</div>
      </div>
      <div class="am-stat">
        <div class="label">已忽略（非代码问题）</div>
        <div class="value">{{ t.ignored || 0 }}</div>
      </div>
      <div class="am-stat">
        <div class="label">今日任务 / 成功</div>
        <div class="value">{{ ov.today?.total || 0 }} / {{ ov.today?.success || 0 }}</div>
      </div>
    </div>

    <el-row :gutter="16">
      <el-col :span="14">
        <div class="am-card">
          <div class="am-toolbar">
            <span style="font-weight:600">任务趋势（近 14 天）</span>
            <div class="am-flex-1" />
            <el-button size="small" @click="$router.push('/statistics')">查看统计</el-button>
          </div>
          <div ref="trendRef" style="height:300px" />
        </div>
      </el-col>
      <el-col :span="10">
        <div class="am-card">
          <div class="am-toolbar"><span style="font-weight:600">任务状态分布</span></div>
          <div ref="pieRef" style="height:300px" />
        </div>
      </el-col>
    </el-row>

    <div class="am-card">
      <div class="am-toolbar">
        <span style="font-weight:600">最近的修复任务</span>
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
    legend: { data: ['成功', '失败', '已忽略', '处理中'], textStyle: { color: '#8b94a7' }, top: 0 },
    grid: { left: 40, right: 20, top: 40, bottom: 30 },
    xAxis: { type: 'category', data: rows.map(r => r.day?.slice(5)), axisLine: { lineStyle: { color: '#262d3d' } }, axisLabel: { color: '#8b94a7' } },
    yAxis: { type: 'value', splitLine: { lineStyle: { color: '#1e2430' } }, axisLabel: { color: '#8b94a7' } },
    series: [
      { name: '成功', type: 'bar', stack: 'a', data: rows.map(r => r.success), itemStyle: { color: '#37b24d' } },
      { name: '失败', type: 'bar', stack: 'a', data: rows.map(r => r.failed), itemStyle: { color: '#f0506e' } },
      { name: '已忽略', type: 'bar', stack: 'a', data: rows.map(r => r.ignored), itemStyle: { color: '#5c6478' } },
      { name: '处理中', type: 'bar', stack: 'a', data: rows.map(r => r.pending), itemStyle: { color: '#4f8cff' } }
    ]
  })
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
    legend: { bottom: 0, textStyle: { color: '#8b94a7' } },
    series: [{
      type: 'pie', radius: ['45%', '68%'], center: ['50%', '45%'], data: items,
      label: { color: '#8b94a7' },
      color: ['#4f8cff', '#37b24d', '#f59f00', '#f0506e', '#5c6478', '#3b82f6']
    }]
  })
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
