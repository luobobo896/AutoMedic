<template>
  <div class="am-page">
    <el-row :gutter="16">
      <el-col :span="9">
        <div class="am-card">
          <div class="am-toolbar">
            <span style="font-weight:600">大模型厂家</span>
            <div class="am-flex-1" />
            <el-button type="primary" size="small" :icon="'Plus'" @click="openProvider">新增厂家</el-button>
          </div>
          <el-table :data="providers" size="small" highlight-current-row @current-change="onSelectProvider">
            <el-table-column prop="name" label="厂家" min-width="140" />
            <el-table-column prop="key" label="标识" width="140" />
            <el-table-column label="Key" width="120">
              <template #default="{ row }">
                <span class="am-mono">{{ row.api_key_masked || '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="70">
              <template #default="{ row }">{{ row.enabled ? '是' : '否' }}</template>
            </el-table-column>
            <el-table-column label="操作" width="120" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click.stop="editProvider(row)">编辑</el-button>
                <el-button link type="danger" @click.stop="removeProvider(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-col>

      <el-col :span="15">
        <div class="am-card">
          <div class="am-toolbar">
            <span style="font-weight:600">模型配置</span>
            <el-tag v-if="current" size="small">{{ current.name }}</el-tag>
            <div class="am-flex-1" />
            <el-button type="primary" size="small" :icon="'Plus'" :disabled="!current" @click="openModel">新增模型</el-button>
          </div>
          <el-alert v-if="!current" type="info" :closable="false" show-icon
            title="请先在左侧选择一个厂家，然后为其配置模型。dsh headless 只接受一个任务参数，模型与上下文大小由系统在调用时通过 patch 层注入。" />
          <el-table v-else :data="currentModels" size="small">
            <el-table-column prop="name" label="模型名称" width="180" />
            <el-table-column prop="slug" label="模型标识" width="180" />
            <el-table-column label="输入上下文" width="120">
              <template #default="{ row }">
                <el-tag size="small" effect="plain">{{ formatTokens(row.input_context) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="输出上下文" width="120">
              <template #default="{ row }">
                <el-tag size="small" effect="plain" type="warning">{{ formatTokens(row.output_context) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="max_turns" label="最大轮次" width="100" />
            <el-table-column label="默认" width="80">
              <template #default="{ row }">
                <el-tag v-if="row.is_default" size="small" type="success">默认</el-tag>
                <el-button v-else link size="small" @click="setDefault(row)">设为默认</el-button>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="80">
              <template #default="{ row }">
                <el-switch v-model="row.enabled" size="small" @change="v => toggleModel(row, v)" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="120" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="editModel(row)">编辑</el-button>
                <el-button link type="danger" @click="removeModel(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <div class="am-card">
          <div class="am-toolbar"><span style="font-weight:600">dsh 调用预览</span></div>
          <div class="am-diff" v-if="current && defaultModel">
            <div class="hunk"># 系统生成的 cordis patch 层（--patch 注入）</div>
            <div v-for="(l, i) in patchPreview" :key="i">{{ l }}</div>
            <div class="hunk" style="margin-top:8px"># 实际调用命令</div>
            <div>dsh --profile headless --patch /tmp/am-dsh-xxx/model.patch.yml "$(cat /tmp/am-dsh-xxx/task.txt)"</div>
            <div class="hunk" style="margin-top:8px"># 环境变量</div>
            <div>DSH_PERMISSION_MODE={{ settings.dsh?.permission_mode || 'workspace-write' }}</div>
            <div>{{ envKeyPreview }}=******（未配置则沿用 dsh 自身凭证）</div>
          </div>
          <el-alert v-else type="info" :closable="false" title="选择厂家并配置启用模型后显示调用预览" />
        </div>
      </el-col>
    </el-row>

    <!-- 厂家对话框 -->
    <el-dialog v-model="providerDialog" :title="providerForm.id ? '编辑厂家' : '新增厂家'" width="560">
      <el-form :model="providerForm" label-width="110px">
        <el-form-item label="厂家名称"><el-input v-model="providerForm.name" placeholder="如：DeepSeek 官方" /></el-form-item>
        <el-form-item label="标识 key">
          <el-input v-model="providerForm.key" placeholder="传给 dsh 的 provider 标识，如 deepseek-official" />
        </el-form-item>
        <el-form-item label="类型 kind">
          <el-select v-model="providerForm.kind" allow-create filterable default-first-option style="width:100%">
            <el-option v-for="k in kinds" :key="k" :label="k" :value="k" />
          </el-select>
        </el-form-item>
        <el-form-item label="Base URL"><el-input v-model="providerForm.base_url" /></el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="providerForm.api_key" type="password" show-password placeholder="留空则使用 dsh 自身凭证" />
        </el-form-item>
        <el-form-item label="上下文上限">
          <el-input-number v-model="providerForm.max_input_context" :min="0" :step="1000" /> 输入
          <el-input-number v-model="providerForm.max_output_context" :min="0" :step="1000" style="margin-left:12px" /> 输出
        </el-form-item>
        <el-form-item label="备注"><el-input v-model="providerForm.remark" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="providerForm.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="providerDialog = false">取消</el-button>
        <el-button type="primary" @click="submitProvider">保存</el-button>
      </template>
    </el-dialog>

    <!-- 模型对话框 -->
    <el-dialog v-model="modelDialog" :title="modelForm.id ? '编辑模型' : '新增模型'" width="600">
      <el-form :model="modelForm" label-width="130px">
        <el-form-item label="所属厂家">
          <el-select v-model="modelForm.provider_id" style="width:100%">
            <el-option v-for="p in providers" :key="p.id" :label="p.name" :value="p.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="模型名称"><el-input v-model="modelForm.name" placeholder="展示用名称" /></el-form-item>
        <el-form-item label="模型标识 slug"><el-input v-model="modelForm.slug" placeholder="传给 dsh 的模型标识" /></el-form-item>
        <el-form-item label="输入上下文">
          <el-select v-model="modelForm.input_context" filterable allow-create default-first-option style="width:100%">
            <el-option v-for="o in ctxOptions" :key="o.v" :label="o.label" :value="o.v" />
          </el-select>
        </el-form-item>
        <el-form-item label="输出上下文">
          <el-select v-model="modelForm.output_context" filterable allow-create default-first-option style="width:100%">
            <el-option v-for="o in ctxOptions" :key="o.v" :label="o.label" :value="o.v" />
          </el-select>
        </el-form-item>
        <el-form-item label="最大推理轮次"><el-input-number v-model="modelForm.max_turns" :min="1" :max="1000" /></el-form-item>
        <el-form-item label="温度"><el-input v-model="modelForm.temperature" placeholder="留空使用模型默认" /></el-form-item>
        <el-form-item label="额外参数">
          <el-input v-model="extraText" type="textarea" :rows="3"
            placeholder='JSON，会合并进 dsh patch 配置，如 {"topP": 0.9}' />
        </el-form-item>
        <el-form-item label="设为默认"><el-switch v-model="modelForm.is_default" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="modelForm.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="modelDialog = false">取消</el-button>
        <el-button type="primary" @click="submitModel">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { listLLMConfig, createProvider, updateProvider, deleteProvider, createModel, updateModel, deleteModel, getSettings } from '@/api'
import { formatTokens } from '@/utils/format'
import { ElMessage, ElMessageBox } from 'element-plus'

const kinds = ['deepseek', 'openai', 'anthropic', 'gemini', 'qwen', 'zhipu', 'moonshot', 'doubao', 'custom']
const ctxOptions = [
  { v: 1048576, label: '1M (1048576)' },
  { v: 512000, label: '500K (512000)' },
  { v: 265000, label: '265K (265000)' },
  { v: 200000, label: '200K (200000)' },
  { v: 131072, label: '128K (131072)' },
  { v: 65536, label: '64K (65536)' },
  { v: 32768, label: '32K (32768)' }
]

const providers = ref([])
const models = ref([])
const settings = ref({})
const current = ref(null)

const providerDialog = ref(false)
const modelDialog = ref(false)
const providerForm = ref({ enabled: true, max_input_context: 1000000, max_output_context: 65536 })
const modelForm = ref({ enabled: true, max_turns: 128, input_context: 131072, output_context: 65536 })
const extraText = ref('')

const currentModels = computed(() => models.value.filter(m => m.provider_id === current.value?.id))
const defaultModel = computed(() => models.value.find(m => m.is_default) || currentModels.value[0])

const patchPreview = computed(() => {
  const m = defaultModel.value
  if (!m) return []
  return [
    '- id: agent-default-model',
    '  config:',
    `    provider: ${current.value?.key}`,
    `    model: ${m.slug}`,
    `    inputContextTokens: ${m.input_context}`,
    `    outputContextTokens: ${m.output_context}`
  ]
})

const envKeyPreview = computed(() => {
  const map = {
    deepseek: 'DEEPSEEK_API_KEY', openai: 'OPENAI_API_KEY', anthropic: 'ANTHROPIC_API_KEY',
    gemini: 'GEMINI_API_KEY', qwen: 'DASHSCOPE_API_KEY', zhipu: 'ZHIPU_API_KEY',
    moonshot: 'MOONSHOT_API_KEY', doubao: 'ARK_API_KEY'
  }
  const k = current.value?.kind
  return map[k] || (current.value?.key || 'PROVIDER').toUpperCase().replace(/-/g, '_') + '_API_KEY'
})

async function load() {
  const r = await listLLMConfig()
  providers.value = r.data?.providers || []
  models.value = r.data?.models || []
  if (!current.value && providers.value.length) current.value = providers.value[0]
}

function onSelectProvider(row) { if (row) current.value = row }

function openProvider() {
  providerForm.value = { enabled: true, max_input_context: 1000000, max_output_context: 65536, kind: 'openai' }
  providerDialog.value = true
}
function editProvider(row) { providerForm.value = { ...row, api_key: '' }; providerDialog.value = true }

async function submitProvider() {
  if (providerForm.value.id) await updateProvider(providerForm.value.id, providerForm.value)
  else await createProvider(providerForm.value)
  ElMessage.success('已保存')
  providerDialog.value = false
  load()
}

async function removeProvider(row) {
  await ElMessageBox.confirm(`确认删除厂家「${row.name}」？`, '警告', { type: 'warning' })
  await deleteProvider(row.id)
  load()
}

function openModel() {
  modelForm.value = { enabled: true, max_turns: 128, input_context: 131072, output_context: 65536, provider_id: current.value?.id }
  extraText.value = ''
  modelDialog.value = true
}
function editModel(row) {
  modelForm.value = { ...row }
  try {
    const o = typeof row.extra_params === 'string' ? JSON.parse(row.extra_params) : row.extra_params
    extraText.value = o && Object.keys(o).length ? JSON.stringify(o, null, 2) : ''
  } catch { extraText.value = '' }
  modelDialog.value = true
}

async function submitModel() {
  const payload = { ...modelForm.value }
  if (extraText.value.trim()) {
    try { payload.extra_params = JSON.parse(extraText.value) } catch {
      ElMessage.error('额外参数不是合法 JSON'); return
    }
  }
  if (payload.id) await updateModel(payload.id, payload)
  else await createModel(payload)
  ElMessage.success('已保存')
  modelDialog.value = false
  load()
}

async function toggleModel(row, v) { await updateModel(row.id, { enabled: v }); ElMessage.success('已更新') }
async function setDefault(row) { await updateModel(row.id, { is_default: true }); ElMessage.success('已设为默认'); load() }

async function removeModel(row) {
  await ElMessageBox.confirm(`确认删除模型「${row.name}」？`, '警告', { type: 'warning' })
  await deleteModel(row.id)
  load()
}

onMounted(async () => {
  try { settings.value = (await getSettings()).data || {} } catch { /* ignore */ }
  load()
})
</script>
