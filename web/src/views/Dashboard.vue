<template>
  <div class="am-page">
    <div class="am-page-head">
      <div>
        <div class="am-page-head__crumb">首页</div>
        <h1 class="am-page-head__title">运营仪表盘</h1>
        <p class="am-page-head__desc">
          告警进来后先看这一屏：进来多少事件、自动修好多少、卡在人工确认还是失败。
        </p>
      </div>
      <div class="am-page-head__actions">
        <el-button :icon="'Refresh'" aria-label="刷新" @click="load" />
        <el-button type="primary" @click="$router.push('/events')">进入事件中心</el-button>
      </div>
    </div>

    <div class="am-stat-grid">
      <div class="am-stat">
        <div class="label">事件总数</div>
        <div class="value">{{ ov.events || 0 }}</div>
        <div class="am-stat__hint">去重后的告警事件</div>
      </div>
      <div class="am-stat">
        <div class="label">修复流程</div>
        <div class="value">{{ t.total || 0 }}</div>
        <div class="am-stat__hint">今日新增 {{ ov.today?.total || 0 }}</div>
      </div>
      <div class="am-stat">
        <div class="label">自愈成功率</div>
        <div class="value am-value--success">{{ successRate }}%</div>
        <div class="am-stat__hint">成功 {{ t.success || 0 }} / 结束 {{ finished || 0 }}</div>
      </div>
      <div class="am-stat">
        <div class="label">待人工确认</div>
        <div class="value am-value--warning">{{ t.confirming || 0 }}</div>
        <div class="am-stat__hint">半自动流程等你点确认</div>
      </div>
      <div class="am-stat">
        <div class="label">失败流程</div>
        <div class="value am-value--danger">{{ t.failed || 0 }}</div>
        <div class="am-stat__hint">可重试或重跑</div>
      </div>
      <div class="am-stat">
        <div class="label">已忽略</div>
        <div class="value">{{ t.ignored || 0 }}</div>
        <div class="am-stat__hint">判定为非代码问题</div>
      </div>
      <div class="am-stat">
        <div class="label">平均自愈耗时</div>
        <div class="value">{{ formatDuration(ov.avg_duration_ms) }}</div>
        <div class="am-stat__hint">含准备与推送</div>
      </div>
      <div class="am-stat">
        <div class="label">今日修复成功</div>
        <div class="value">{{ ov.today?.success || 0 }}</div>
        <div class="am-stat__hint">今日流程 {{ ov.today?.total || 0 }}</div>
      </div>
    </div>

    <div class="am-card">
      <div class="am-toolbar">
        <span class="am-card__title">最近流程</span>
        <span class="am-text-dim am-hint">点任意一行进入流程监控，看卡在哪一步</span>
        <div class="am-flex-1" />
        <el-button size="small" @click="$router.push('/tasks')">全部流程</el-button>
      </div>
      <el-table :data="tasks" v-loading="loading" size="small" @row-click="row => $router.push('/tasks/' + row.id)">
        <el-table-column label="事件号" width="176">
          <template #default="{ row }">
            <router-link
              v-if="row.event"
              class="am-link am-mono"
              :to="{ path: '/events', query: { code: eventCode(row.event) } }"
              @click.stop
            >
              {{ eventCode(row.event) }}
            </router-link>
            <span v-else class="am-mono am-text-faint">手动 #{{ row.id }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="130">
          <template #default="{ row }">
            <el-tag size="small" :type="STATUS_META[row.status]?.type">{{ STATUS_META[row.status]?.label }}</el-tag>
            <div class="am-text-dim am-hint">流程 #{{ row.id }} · {{ STAGE_LABEL[row.stage] || '' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="触发事件" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">{{ row.event?.title || '手动触发' }}</template>
        </el-table-column>
        <el-table-column label="模式" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.mode === 'auto' ? 'danger' : 'warning'">
              {{ row.mode === 'auto' ? '全自动' : '半自动' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Agent 模型" width="150">
          <template #default="{ row }">{{ row.model?.name || row.dsh_model || '-' }}</template>
        </el-table-column>
        <el-table-column label="耗时" width="80">
          <template #default="{ row }">{{ formatDuration(row.duration_ms) }}</template>
        </el-table-column>
        <el-table-column label="创建时间" width="140">
          <template #default="{ row }">{{ formatTimeShort(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="96">
          <template #default="{ row }">
            <el-button link type="primary" @click.stop="$router.push('/tasks/' + row.id)">进入监控</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <div class="am-empty">
            <div class="am-empty__title">还没有修复流程</div>
            <div class="am-empty__desc">
              告警源按《采集接入》投递事件，命中规则后这里会出现第一条流程。
            </div>
            <div class="am-empty__actions">
              <el-button size="small" @click="$router.push('/rules')">配置修复规则</el-button>
              <el-button size="small" @click="$router.push('/tokens')">生成接入令牌</el-button>
            </div>
          </div>
        </template>
      </el-table>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { overview, listTasks } from '@/api'
import { STATUS_META, STAGE_LABEL, formatDuration, formatTimeShort, eventCode } from '@/utils/format'

const router = useRouter()
const ov = ref({ tasks: {}, today: {} })
const tasks = ref([])
const loading = ref(false)

const t = computed(() => ov.value.tasks || {})
// 成功率分母用「已结束流程」（成功+失败+忽略），比全量任务更贴近自愈口径
const finished = computed(() => (t.value.success || 0) + (t.value.failed || 0) + (t.value.ignored || 0))
const successRate = computed(() => {
  if (!finished.value) return 0
  return Math.round((t.value.success || 0) / finished.value * 100)
})

async function load() {
  loading.value = true
  try {
    const [o, tk] = await Promise.all([
      overview(),
      listTasks({ page: 1, page_size: 10 })
    ])
    ov.value = o.data || o
    tasks.value = tk.data?.list || []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
