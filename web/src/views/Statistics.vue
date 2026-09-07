<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar">
        <span style="font-weight:600">时间范围</span>
        <el-select v-model="days" style="width:120px" @change="loadAll">
          <el-option label="近 7 天" :value="7" />
          <el-option label="近 14 天" :value="14" />
          <el-option label="近 30 天" :value="30" />
          <el-option label="近 90 天" :value="90" />
        </el-select>
        <el-select v-model="projectId" clearable placeholder="全部项目" style="width:180px" @change="loadAll">
          <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
        <el-select v-model="repoId" clearable placeholder="全部仓库" style="width:180px" @change="loadAll">
          <el-option v-for="r in repos" :key="r.id" :label="r.name" :value="r.id" />
        </el-select>
        <div class="am-flex-1" />
        <el-button :icon="'Refresh'" @click="loadAll" />
      </div>

      <div class="am-stat-grid">
        <div class="am-stat">
          <div class="label">任务总数</div>
          <div class="value">{{ ov.total || 0 }}</div>
        </div>
        <div class="am-stat">
          <div class="label">修复成功率</div>
          <div class="value" style="color:#37b24d">{{ ov.success_rate || 0 }}%</div>
        </div>
        <div class="am-stat">
          <div class="label">失败</div>
          <div class="value" style="color:#f0506e">{{ ov.failed || 0 }}</div>
        </div>
        <div class="am-stat">
          <div class="label">已忽略（非代码问题）</div>
          <div class="value">{{ ov.ignored || 0 }}</div>
        </div>
        <div class="am-stat">
          <div class="label">待人工确认</div>
          <div class="value" style="color:#f59f00">{{ ov.confirming || 0 }}</div>
        </div>
        <div class="am-stat">
          <div class="label">平均耗时</div>
          <div class="value">{{ formatDuration(ov.avg_duration_ms) }}</div>
        </div>
      </div>
    </div>

    <el-row :gutter="16">
      <el-col :span="16">
        <div class="am-card">
          <div class="am-toolbar"><span style="font-weight:600">修复趋势</span></div>
          <div ref="trendRef" style="height:320px" />
        </div>
      </el-col>
      <el-col :span="8">
        <div class="am-card">
          <div class="am-toolbar"><span style="font-weight:600">状态分布</span></div>
          <div ref="pieRef" style="height:320px" />
        </div>
      </el-col>
    </el-row>

    <el-row :gutter="16">
      <el-col :span="12">
        <div class="am-card">
          <div class="am-toolbar">
            <span style="font-weight:600">按项目统计</span>
          </div>
          <div ref="projectRef" style="height:300px" />
        </div>
      </el-col>
      <el-col :span="12">
        <div class="am-card">
          <div class="am-toolbar">
            <span style="font-weight:600">按维度统计</span>
            <el-select v-model="group" size="small" style="width:130px" @change="loadGroup">
              <el-option label="仓库" value="repo" />
              <el-option label="触发规则" value="rule" />
              <el-option label="事件来源" value="source" />
              <el-option label="模型" value="model" />
            </el-select>
          </div>
          <div ref="groupRef" style="height:300px" />
        </div>
      </el-col>
    </el-row>

    <div class="am-card">
      <div class="am-toolbar"><span style="font-weight:600">明细数据</span></div>
      <el-table :data="byProject" size="small">
        <el-table-column prop="name" label="项目" min-width="160" />
        <el-table-column prop="total" label="任务数" width="100" />
        <el-table-column prop="success" label="成功" width="90" />
        <el-table-column prop="failed" label="失败" width="90" />
        <el-table-column prop="ignored" label="已忽略" width="90" />
        <el-table-column label="成功率" width="110">
          <template #default="{ row }">{{ row.total ? Math.round(row.success / row.total * 100) : 0 }}%</template>
        </el-table-column>
        <el-table-column label="平均耗时" width="120">
          <template #default="{ row }">{{ formatDuration(row.avg_ms) }}</template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { statsOverview, statsTrend, statsGroup, listProjects, listRepos } from '@/api'
import { formatDuration } from '@/utils/format'
import * as echarts from 'echarts'

const days = ref(14)
const group = ref('repo')
const projectId = ref('')
const repoId = ref('')
const projects = ref([])
const repos = ref([])
const ov = ref({})
const byProject = ref([])

const trendRef = ref()
const pieRef = ref()
const projectRef = ref()
const groupRef = ref()
let trendChart, pieChart, projectChart, groupChart

const params = () => {
  const p = { days: days.value }
  if (projectId.value) p.project_id = projectId.value
  if (repoId.value) p.repo_id = repoId.value
  return p
}

async function loadAll() {
  const [o, tr, gp] = await Promise.all([statsOverview(params()), statsTrend(params()), statsGroup({ ...params(), group: 'project' })])
  ov.value = o.data || {}
  byProject.value = gp.data || []
  renderTrend(tr.data || [])
  renderPie()
  renderProject()
  loadGroup()
}

async function loadGroup() {
  const r = await statsGroup({ ...params(), group: group.value })
  renderGroup(r.data || [])
}

function baseAxis(rows) {
  return {
    type: 'category',
    data: rows.map(r => r.name || r.key),
    axisLine: { lineStyle: { color: '#262d3d' } },
    axisLabel: { color: '#8b94a7', interval: 0, rotate: rows.length > 6 ? 25 : 0 }
  }
}

function renderTrend(rows) {
  nextTick(() => {
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
    }, true)
  })
}

function renderPie() {
  nextTick(() => {
    if (!pieRef.value) return
    pieChart = pieChart || echarts.init(pieRef.value)
    const data = [
      { name: '成功', value: ov.value.success || 0, itemStyle: { color: '#37b24d' } },
      { name: '失败', value: ov.value.failed || 0, itemStyle: { color: '#f0506e' } },
      { name: '已忽略', value: ov.value.ignored || 0, itemStyle: { color: '#5c6478' } },
      { name: '待确认', value: ov.value.confirming || 0, itemStyle: { color: '#f59f00' } },
      { name: '待处理', value: ov.value.pending || 0, itemStyle: { color: '#4f8cff' } }
    ].filter(d => d.value > 0)
    pieChart.setOption({
      backgroundColor: 'transparent',
      tooltip: { trigger: 'item' },
      legend: { bottom: 0, textStyle: { color: '#8b94a7' } },
      series: [{ type: 'pie', radius: ['42%', '66%'], center: ['50%', '44%'], data, label: { color: '#8b94a7' } }]
    }, true)
  })
}

function renderProject() {
  nextTick(() => {
    if (!projectRef.value) return
    projectChart = projectChart || echarts.init(projectRef.value)
    projectChart.setOption({
      backgroundColor: 'transparent',
      tooltip: { trigger: 'axis' },
      legend: { data: ['成功', '失败', '已忽略'], textStyle: { color: '#8b94a7' }, top: 0 },
      grid: { left: 40, right: 20, top: 40, bottom: 40 },
      xAxis: baseAxis(byProject.value),
      yAxis: { type: 'value', splitLine: { lineStyle: { color: '#1e2430' } }, axisLabel: { color: '#8b94a7' } },
      series: [
        { name: '成功', type: 'bar', stack: 'a', data: byProject.value.map(r => r.success), itemStyle: { color: '#37b24d' } },
        { name: '失败', type: 'bar', stack: 'a', data: byProject.value.map(r => r.failed), itemStyle: { color: '#f0506e' } },
        { name: '已忽略', type: 'bar', stack: 'a', data: byProject.value.map(r => r.ignored), itemStyle: { color: '#5c6478' } }
      ]
    }, true)
  })
}

function renderGroup(rows) {
  nextTick(() => {
    if (!groupRef.value) return
    groupChart = groupChart || echarts.init(groupRef.value)
    groupChart.setOption({
      backgroundColor: 'transparent',
      tooltip: { trigger: 'axis' },
      legend: { data: ['成功', '失败', '已忽略'], textStyle: { color: '#8b94a7' }, top: 0 },
      grid: { left: 40, right: 20, top: 40, bottom: 40 },
      xAxis: baseAxis(rows),
      yAxis: { type: 'value', splitLine: { lineStyle: { color: '#1e2430' } }, axisLabel: { color: '#8b94a7' } },
      series: [
        { name: '成功', type: 'bar', stack: 'a', data: rows.map(r => r.success), itemStyle: { color: '#37b24d' } },
        { name: '失败', type: 'bar', stack: 'a', data: rows.map(r => r.failed), itemStyle: { color: '#f0506e' } },
        { name: '已忽略', type: 'bar', stack: 'a', data: rows.map(r => r.ignored), itemStyle: { color: '#5c6478' } }
      ]
    }, true)
  })
}

function onResize() {
  trendChart?.resize(); pieChart?.resize(); projectChart?.resize(); groupChart?.resize()
}

onMounted(async () => {
  const [p, r] = await Promise.all([listProjects({ page_size: 100 }), listRepos({ page_size: 200 })])
  projects.value = p.data?.list || []
  repos.value = r.data?.list || []
  loadAll()
  window.addEventListener('resize', onResize)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  ;[trendChart, pieChart, projectChart, groupChart].forEach(c => c?.dispose())
})
</script>
