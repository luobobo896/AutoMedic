<template>
  <div class="am-page am-page-model">
    <div class="am-page-head">
      <div>
        <h2 class="am-page-title">模型配置</h2>
        <p class="am-page-desc">厂家只配接入信息；输入/输出上下文在每个模型上设置，调用 dsh 时通过 patch 注入。</p>
      </div>
    </div>

    <!-- 厂家 -->
    <section class="am-section">
      <div class="am-section-head">
        <div>
          <h3 class="am-section-title">大模型厂家</h3>
          <p class="am-section-desc">点击卡片切换当前使用的厂家。</p>
        </div>
        <el-button class="am-btn-soft" :icon="'Plus'" @click="openProvider">新增厂家</el-button>
      </div>
      <div class="am-card-grid">
        <div v-for="p in providers" :key="p.id" class="am-provider-card"
          :class="{ 'is-active': current?.id === p.id }" @click="onSelectProvider(p)">
          <div class="am-provider-top">
            <span class="am-provider-name">{{ p.name }}</span>
            <el-tag v-if="current?.id === p.id" class="am-pill am-pill-current" size="small">当前</el-tag>
            <div class="am-flex-1" />
            <el-button class="am-icon-btn" link @click.stop="editProvider(p)">
              <el-icon><EditPen /></el-icon>
            </el-button>
            <el-button class="am-icon-btn am-icon-danger" link @click.stop="removeProvider(p)">
              <el-icon><Delete /></el-icon>
            </el-button>
          </div>
          <div class="am-provider-url am-mono">{{ p.base_url || '-' }}</div>
          <div class="am-provider-meta">
            <span class="am-pill">{{ p.kind || 'custom' }}</span>
            <span class="am-pill">{{ p.key }}</span>
            <span class="am-provider-count">{{ providerModelCount(p.id) }} 个模型</span>
          </div>
        </div>
        <div v-if="!providers.length" class="am-card am-empty">暂无厂家，点击右上角「新增厂家」创建。</div>
      </div>
    </section>

    <!-- 模型 -->
    <section class="am-section">
      <div class="am-section-head">
        <div>
          <h3 class="am-section-title">模型列表<span v-if="current" class="am-section-suffix">· {{ current.name }}</span></h3>
          <p class="am-section-desc">输出上下文即最大输出长度，选项按模型官方规格过滤，超出能力范围的档位不可选。</p>
        </div>
        <el-button class="am-btn-soft" :icon="'Plus'" :disabled="!current" @click="openModel">新增模型</el-button>
      </div>
      <el-alert v-if="!current" type="info" :closable="false" show-icon
        title="请先在上方选择一个厂家，然后为其配置模型。" class="am-alert" />
      <div v-else class="am-model-list">
        <div v-for="m in currentModels" :key="m.id" class="am-model-row">
          <div class="am-model-info">
            <div class="am-model-name">
              {{ m.name }}
              <el-tag v-if="m.is_default" class="am-pill am-pill-current" size="small">默认</el-tag>
            </div>
            <div class="am-model-sub">
              <span class="am-mono">{{ m.slug }}</span>
              <span class="am-pill">输入 {{ formatTokens(m.input_context) }}</span>
              <span class="am-pill am-pill-accent">输出 {{ formatTokens(m.output_context) }}</span>
              <span class="am-model-dim">最大 {{ m.max_turns }} 轮</span>
            </div>
          </div>
          <div class="am-model-actions">
            <el-button v-if="!m.is_default" link size="small" @click="setDefault(m)">设为默认</el-button>
            <el-switch v-model="m.enabled" size="small" @change="v => toggleModel(m, v)" />
            <el-button class="am-icon-btn" link @click="editModel(m)">
              <el-icon><EditPen /></el-icon>
            </el-button>
            <el-button class="am-icon-btn am-icon-danger" link @click="removeModel(m)">
              <el-icon><Delete /></el-icon>
            </el-button>
          </div>
        </div>
        <div v-if="!currentModels.length" class="am-card am-empty">该厂家下暂无模型。</div>
      </div>
    </section>

    <!-- dsh 调用预览 -->
    <section class="am-section">
      <div class="am-section-head">
        <div>
          <h3 class="am-section-title">dsh 调用预览</h3>
          <p class="am-section-desc">系统生成的 cordis patch 层与实际调用命令。</p>
        </div>
      </div>
      <div class="am-diff" v-if="current && defaultModel">
        <div class="hunk"># 系统生成的 cordis patch 层（--patch 注入）</div>
        <div v-for="(l, i) in patchPreview" :key="i">{{ l }}</div>
        <div class="hunk" style="margin-top:8px"># 实际调用命令</div>
        <div>dsh --profile headless --patch /tmp/am-dsh-xxx/model.patch.yml "$(cat /tmp/am-dsh-xxx/task.txt)"</div>
        <div class="hunk" style="margin-top:8px"># 环境变量</div>
        <div>DSH_PERMISSION_MODE={{ settings.dsh?.permission_mode || 'workspace-write' }}</div>
        <div>{{ envKeyPreview }}=******（未配置则沿用 dsh 自身凭证）</div>
      </div>
      <div v-else class="am-card am-empty">选择厂家并配置启用模型后显示调用预览。</div>
    </section>

    <!-- 厂家对话框 -->
    <el-dialog v-model="providerDialog" :title="providerForm.id ? '编辑厂家' : '新增厂家'" width="560" class="am-dialog">
      <p class="am-dialog-desc">厂家对接 OpenAI 兼容 API，API Key 加密存储。</p>
      <el-form :model="providerForm" label-position="top" class="am-form">
        <el-form-item label="类型">
          <el-select v-model="providerForm.kind" style="width:100%" @change="onProviderKind">
            <el-option v-for="p in dict.providerPresets()" :key="p.kind" :label="p.name" :value="p.kind" />
          </el-select>
        </el-form-item>
        <el-form-item label="厂家名称"><el-input v-model="providerForm.name" placeholder="展示名称" /></el-form-item>
        <el-form-item label="标识 key">
          <el-input v-model="providerForm.key" placeholder="传给 dsh 的 provider 标识" />
        </el-form-item>
        <el-form-item label="Base URL"><el-input v-model="providerForm.base_url" placeholder="选类型后自动填，自定义可改" /></el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="providerForm.api_key" type="password" show-password placeholder="留空则使用 dsh 自身凭证" />
        </el-form-item>
        <el-form-item label="备注"><el-input v-model="providerForm.remark" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="providerForm.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button class="am-btn-soft" @click="providerDialog = false">取消</el-button>
        <el-button type="primary" class="am-btn-primary" @click="submitProvider">保存环境</el-button>
      </template>
    </el-dialog>

    <!-- 模型对话框 -->
    <el-dialog v-model="modelDialog" :title="modelForm.id ? '编辑模型' : '新增模型'" width="620" class="am-dialog">
      <p class="am-dialog-desc">输出长度档位按模型官方文档声明过滤；384K 仅对官方支持 384K 输出的模型开放。</p>
      <el-form :model="modelForm" label-position="top" class="am-form">
        <div class="am-form-grid">
          <el-form-item label="所属厂家">
            <el-select v-model="modelForm.provider_id" style="width:100%">
              <el-option v-for="p in providers" :key="p.id" :label="p.name" :value="p.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="模型标识 slug">
            <el-select v-model="modelForm.slug" filterable allow-create default-first-option style="width:100%"
              placeholder="选择或输入官方模型名" @change="onSlugChange">
              <el-option v-for="s in slugOptions" :key="s" :label="s" :value="s" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="模型名称"><el-input v-model="modelForm.name" placeholder="展示用名称" /></el-form-item>

        <div class="am-form-divider">上下文设置</div>
        <div class="am-form-grid">
          <el-form-item label="上下文窗口（输入）">
            <el-select v-model="modelForm.input_context" filterable allow-create default-first-option style="width:100%">
              <el-option v-for="o in inputOptions" :key="o.v" :label="o.label" :value="o.v" />
            </el-select>
          </el-form-item>
          <el-form-item label="输出长度（最大输出）">
            <el-select v-model="modelForm.output_context" filterable allow-create default-first-option style="width:100%">
              <el-option v-for="o in outputOptions" :key="o.v" :label="o.label" :value="o.v">
                <span class="am-opt">{{ o.label }}<span class="am-opt-tokens">{{ o.tokens }} tokens</span></span>
              </el-option>
            </el-select>
          </el-form-item>
        </div>
        <div class="am-field-hint">
          <span>1K = 1,024 tokens，选项按「K（tokens）」标注。</span>
          <span v-if="specInfo" class="am-field-hint-spec">
            <el-icon><InfoFilled /></el-icon>
            <el-tooltip placement="top" :content="specInfo.note">
              <span class="am-hint-link">{{ currentSpecLabel }}</span>
            </el-tooltip>
          </span>
        </div>
        <el-alert v-if="compat384Note" type="warning" :closable="false" show-icon class="am-alert"
          :title="compat384Note" />

        <el-collapse class="am-collapse">
          <el-collapse-item name="adv">
            <template #title>
              <div class="am-collapse-title">
                <span>高级设置</span>
                <span class="am-collapse-desc">一般无需修改，配置异常或排错时再调整</span>
              </div>
            </template>
            <el-form-item label="最大推理轮次"><el-input-number v-model="modelForm.max_turns" :min="1" :max="1000" /></el-form-item>
            <el-form-item label="温度">
              <el-select v-model="modelForm.temperature" style="width:100%">
                <el-option v-for="o in tempOptions" :key="o.value || 'default'" :label="o.label" :value="o.value" />
              </el-select>
            </el-form-item>
            <el-form-item label="额外参数">
              <el-input v-model="extraText" type="textarea" :rows="3"
                placeholder='JSON，会合并进 dsh patch 配置，如 {"topP": 0.9}' />
            </el-form-item>
            <div class="am-form-grid">
              <el-form-item label="设为默认"><el-switch v-model="modelForm.is_default" /></el-form-item>
              <el-form-item label="启用"><el-switch v-model="modelForm.enabled" /></el-form-item>
            </div>
          </el-collapse-item>
        </el-collapse>
      </el-form>
      <template #footer>
        <el-button class="am-btn-soft" @click="modelDialog = false">取消</el-button>
        <el-button type="primary" class="am-btn-primary" @click="submitModel">保存模型</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { listLLMConfig, createProvider, updateProvider, deleteProvider, createModel, updateModel, deleteModel, getSettings } from '@/api'
import { formatTokens } from '@/utils/format'
import { ElMessage, ElMessageBox } from 'element-plus'
import { EditPen, Delete, InfoFilled } from '@element-plus/icons-vue'
import { useDicts } from '@/composables/useDicts'

// ===== 长度档位池（1K = 1024 tokens；个别为厂商十进制标称值，括号内为精确 tokens）=====
const LEN_POOL = [
  { v: 1048576, k: '1M', tokens: '1,048,576' },
  { v: 524288, k: '512K', tokens: '524,288' },
  { v: 393216, k: '384K', tokens: '393,216', out: true },
  { v: 262144, k: '256K', tokens: '262,144' },
  { v: 200000, k: '200K', tokens: '200,000', decimal: true },
  { v: 131072, k: '128K', tokens: '131,072' },
  { v: 128000, k: '128K', tokens: '128,000', decimal: true },
  { v: 65536, k: '64K', tokens: '65,536' },
  { v: 32768, k: '32K', tokens: '32,768' },
  { v: 16384, k: '16K', tokens: '16,384' },
  { v: 8192, k: '8K', tokens: '8,192' }
]
const label = o => `${o.k}（${o.tokens} tokens）`

// ===== 模型官方输出规格表（来源：各平台官方文档，2026-09 校对）=====
// exact384: 官方明确声明“最大输出 384K”；within: 384K 在官方最大输出范围内（兼容可用，需提示）
const MODEL_SPECS = [
  { match: /^deepseek-v4/, maxOutput: 393216, maxInput: 1048576, exact384: true,
    note: 'DeepSeek 官方文档：V4 系列（flash / pro / flash-vision-exp）上下文 1M，最大输出 384K（393,216 tokens），思考内容计入输出预算。' },
  { match: /^deepseek-v4-pro-ga/, maxOutput: 393216, maxInput: 1048576, exact384: true,
    note: '火山引擎托管 DeepSeek-V4-Pro 正式版：上下文 1024K，最大输出 384K（兼容 OpenAI / Anthropic 两种 API 形态）。' },
  { match: /^deepseek-(chat|reasoner)/, maxOutput: 65536, maxInput: 131072,
    note: '旧模型名 deepseek-chat / deepseek-reasoner：最大输出 64K，官方已于 2026-07 停用，建议迁移至 deepseek-v4-*。' },
  { match: /^kimi-k3/, maxOutput: 1048576, maxInput: 1048576, within384: true,
    note: 'Moonshot 官方文档：Kimi K3 最大输出上限 1,048,576 tokens（整个上下文窗口）。384K 未被官方单列为档位，但在支持范围内（兼容档位）。' },
  { match: /^kimi-k2/, maxOutput: 262144, maxInput: 262144,
    note: 'Moonshot / SiliconFlow 文档：Kimi K2 系列最大输出 256K。' },
  { match: /^(gpt-5|o3|o4)/, maxOutput: 128000, maxInput: 400000,
    note: 'OpenAI 官方：当前推理系列（GPT-5.x / o 系）最大输出统一为 128,000 tokens。' },
  { match: /^gpt-4o/, maxOutput: 16384, maxInput: 128000,
    note: 'OpenAI 官方：GPT-4o 系列最大输出 16,384 tokens。' },
  { match: /^claude-opus|^claude-sonnet|^claude-fable/, maxOutput: 128000, maxInput: 1000000,
    note: 'Anthropic 官方：Opus / Sonnet 5 系最大输出 128K（1M 上下文）；扩展思考同样消耗输出预算。' },
  { match: /^claude-haiku/, maxOutput: 65536, maxInput: 200000,
    note: 'Anthropic 官方：Haiku 4.5 最大输出 64K。' },
  { match: /^gemini/, maxOutput: 65536, maxInput: 1048576,
    note: 'Google 官方：Gemini 文本模型最大输出统一为 65,536 tokens。' },
  { match: /^qwen3\.\d-max|qwen.*-max/, maxOutput: 131072, maxInput: 1048576,
    note: '阿里云百炼官方：Qwen Max 系列最大输出 128K（131,072 tokens）。' },
  { match: /^qwen/, maxOutput: 65536, maxInput: 1048576,
    note: '阿里云百炼官方：Qwen 通用系列最大输出 64K。' },
  { match: /^glm-?5/, maxOutput: 131072, maxInput: 1048576,
    note: '智谱官方：GLM-5 系列最大输出 128K。' },
  { match: /^grok/, maxOutput: 131072, maxInput: 2000000,
    note: 'xAI 官方：Grok 系列最大输出 128K。' },
  { match: /^minimax-m/, maxOutput: 524288, maxInput: 1048576,
    note: 'MiniMax 官方：M 系列推荐输出 128K，最大 512K。' },
  { match: /^doubao-seed/, maxOutput: 32768, maxInput: 256000,
    note: '火山引擎官方：豆包 Seed 系列最大回答 32K（默认仅 4K，需显式指定）。' }
]

function matchSpec(slug, providerKind) {
  const s = String(slug || '').toLowerCase()
  if (!s) return null
  // 火山托管的 deepseek-v4-pro-ga-* 优先命中专用条目
  const hit = MODEL_SPECS.find(sp => sp.match.test(s))
  if (hit) return hit
  // 未登记的模型：按厂家 kind 给保守上限
  const kindCeil = { deepseek: 65536, openai: 128000, anthropic: 128000, gemini: 65536, qwen: 65536, moonshot: 262144, zhipu: 131072, doubao: 32768 }
  if (providerKind && kindCeil[providerKind]) {
    return { match: /.^/, maxOutput: kindCeil[providerKind],
      note: `该模型未在官方规格表中登记，已按 ${providerKind} 平台公开的最大输出上限做保守过滤；可通过输入框手工指定。` }
  }
  return null
}

// ===== state =====
const dict = useDicts()
const providers = ref([])
const models = ref([])
const settings = ref({})
const current = ref(null)

const providerDialog = ref(false)
const modelDialog = ref(false)
const providerForm = ref({ enabled: true, kind: 'deepseek' })
const modelForm = ref({ enabled: true, max_turns: 128, input_context: 131072, output_context: 65536 })
const extraText = ref('')

const currentModels = computed(() => models.value.filter(m => m.provider_id === current.value?.id))
const defaultModel = computed(() => models.value.find(m => m.is_default) || currentModels.value[0])
const providerModelCount = pid => models.value.filter(m => m.provider_id === pid).length

const modelProviderKind = computed(() =>
  providers.value.find(p => p.id === modelForm.value.provider_id)?.kind || current.value?.kind || '')
const slugOptions = computed(() => dict.slugsForKind(modelProviderKind.value))
const tempOptions = computed(() => dict.options('temperature').map(o => ({
  value: o.value === 'default' ? '' : o.value,
  label: o.label
})))

const specInfo = computed(() => matchSpec(modelForm.value.slug, modelProviderKind.value))
const currentSpecLabel = computed(() => {
  if (!specInfo.value) return ''
  const max = specInfo.value.maxOutput
  const o = LEN_POOL.find(x => x.v === max)
  return `官方规格：最大输出 ${o ? o.k : max + ' tokens'}，已过滤不可用档位`
})

// 输入上下文选项：按官方输入上限过滤，未登记则给全量
const inputOptions = computed(() => {
  const max = specInfo.value?.maxInput
  if (!max) return LEN_POOL.filter(o => !o.out).map(o => ({ ...o, label: label(o) }))
  return LEN_POOL.filter(o => !o.out && o.v <= max).map(o => ({ ...o, label: label(o) }))
})

// 输出长度选项：只展示 ≤ 官方最大输出的档位；384K 仅对官方支持的模型出现
const outputOptions = computed(() => {
  const spec = specInfo.value
  if (!spec) {
    // 未登记且无 kind 上限：保守起见不提供 384K 及以上，可手工输入
    return LEN_POOL.filter(o => o.v !== 393216).map(o => ({ ...o, label: label(o) }))
  }
  return LEN_POOL.filter(o => o.v <= spec.maxOutput).map(o => ({ ...o, label: label(o) }))
})

// 384K 兼容提示（要求 4）：官方未单列 384K 档、但最大输出覆盖 384K 的模型
const compat384Note = computed(() => {
  const spec = specInfo.value
  if (!spec || !spec.within384) return ''
  if (modelForm.value.output_context !== 393216) return ''
  return `该模型官方未单列 384K 输出档位，但其最大输出上限（${spec.maxOutput} tokens）覆盖 384K，属兼容可用档位。`
})

// 选型超出官方上限时自动回退，避免保存超规格值
watch(() => [modelForm.value.slug, modelForm.value.provider_id], () => {
  const spec = specInfo.value
  if (!spec) return
  if (modelForm.value.output_context > spec.maxOutput) modelForm.value.output_context = spec.maxOutput
  if (spec.maxInput && modelForm.value.input_context > spec.maxInput) modelForm.value.input_context = spec.maxInput
})

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

function onSelectProvider(row) { current.value = row }

function onProviderKind(kind) {
  const preset = dict.providerPresets().find(p => p.kind === kind)
  if (!preset) return
  if (!providerForm.value.id) {
    providerForm.value.name = preset.name
    providerForm.value.key = preset.key
    providerForm.value.base_url = preset.base_url
  } else if (!providerForm.value.base_url) {
    providerForm.value.base_url = preset.base_url
  }
}

function openProvider() {
  const preset = dict.providerPresets()[0] || { kind: 'custom', name: '', key: '', base_url: '' }
  providerForm.value = {
    enabled: true, kind: preset.kind, name: preset.name, key: preset.key, base_url: preset.base_url
  }
  providerDialog.value = true
}
function editProvider(row) {
  providerForm.value = {
    id: row.id, name: row.name, key: row.key, kind: row.kind, base_url: row.base_url,
    remark: row.remark, enabled: row.enabled, api_key: ''
  }
  providerDialog.value = true
}

async function submitProvider() {
  const payload = {
    name: providerForm.value.name, key: providerForm.value.key, kind: providerForm.value.kind,
    base_url: providerForm.value.base_url || '', remark: providerForm.value.remark || '',
    enabled: providerForm.value.enabled !== false, api_key: providerForm.value.api_key || ''
  }
  if (providerForm.value.id) await updateProvider(providerForm.value.id, payload)
  else await createProvider(payload)
  ElMessage.success('已保存')
  providerDialog.value = false
  load()
}

async function removeProvider(row) {
  const ok = await ElMessageBox.confirm(`确认删除厂家「${row.name}」？`, '警告', { type: 'warning' }).catch(() => false)
  if (!ok) return
  await deleteProvider(row.id)
  load()
}

function onSlugChange(slug) {
  const slugs = dict.slugsForKind(modelProviderKind.value)
  if (!modelForm.value.name || slugs.includes(modelForm.value.name)) {
    modelForm.value.name = slug
  }
}

function openModel() {
  const slugs = dict.slugsForKind(current.value?.kind)
  modelForm.value = {
    enabled: true, max_turns: 128, input_context: 131072, output_context: 65536,
    provider_id: current.value?.id, slug: slugs[0] || '', name: slugs[0] || '', temperature: ''
  }
  extraText.value = ''
  modelDialog.value = true
}
function editModel(row) {
  modelForm.value = {
    id: row.id,
    provider_id: row.provider_id,
    name: row.name,
    slug: row.slug,
    input_context: row.input_context,
    output_context: row.output_context,
    max_turns: row.max_turns,
    temperature: row.temperature,
    enabled: row.enabled,
    is_default: row.is_default
  }
  try {
    const o = typeof row.extra_params === 'string' ? JSON.parse(row.extra_params) : row.extra_params
    extraText.value = o && Object.keys(o).length ? JSON.stringify(o, null, 2) : ''
  } catch { extraText.value = '' }
  modelDialog.value = true
}

function modelPayload() {
  const payload = {
    provider_id: modelForm.value.provider_id,
    name: modelForm.value.name,
    slug: modelForm.value.slug,
    input_context: Number(modelForm.value.input_context),
    output_context: Number(modelForm.value.output_context),
    max_turns: Number(modelForm.value.max_turns) || 128,
    temperature: modelForm.value.temperature || '',
    enabled: !!modelForm.value.enabled,
    is_default: !!modelForm.value.is_default
  }
  if (extraText.value.trim()) {
    payload.extra_params = JSON.parse(extraText.value)
  }
  return payload
}

async function submitModel() {
  let payload
  try {
    payload = modelPayload()
  } catch {
    ElMessage.error('额外参数不是合法 JSON')
    return
  }
  if (modelForm.value.id) await updateModel(modelForm.value.id, payload)
  else await createModel(payload)
  ElMessage.success('已保存')
  modelDialog.value = false
  load()
}

async function toggleModel(row, v) { await updateModel(row.id, { enabled: v }); ElMessage.success('已更新') }
async function setDefault(row) { await updateModel(row.id, { is_default: true }); ElMessage.success('已设为默认'); load() }

async function removeModel(row) {
  const ok = await ElMessageBox.confirm(`确认删除模型「${row.name}」？`, '警告', { type: 'warning' }).catch(() => false)
  if (!ok) return
  await deleteModel(row.id)
  load()
}

onMounted(async () => {
  try { settings.value = (await getSettings()).data || {} } catch { /* ignore */ }
  await dict.load()
  load()
})
</script>

<style scoped>
.am-page-model { max-width: 1200px; margin: 0 auto; padding: 28px 24px 48px; }
.am-page-head { margin-bottom: 24px; }
.am-page-title { margin: 0; font-size: 22px; font-weight: 700; letter-spacing: .2px; }
.am-page-desc { margin: 6px 0 0; font-size: 13px; color: var(--am-text-dim); line-height: 1.6; }

.am-section { margin-bottom: 28px; }
.am-section-head { display: flex; align-items: flex-end; gap: 12px; margin-bottom: 12px; }
.am-section-title { margin: 0; font-size: 16px; font-weight: 600; }
.am-section-suffix { margin-left: 8px; font-weight: 400; color: var(--am-text-dim); }
.am-section-desc { margin: 4px 0 0; font-size: 12.5px; color: var(--am-text-dim); line-height: 1.5; }
.am-section-head .am-flex-1, .am-section-head .el-button { margin-left: auto; }

/* 厂家卡片 */
.am-card-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 14px; }
.am-provider-card {
  background: var(--am-bg-elevated);
  border: 1px solid var(--am-border);
  border-radius: var(--am-radius);
  padding: 16px 18px;
  cursor: pointer;
  transition: border-color .18s ease, box-shadow .18s ease, transform .18s ease;
}
.am-provider-card:hover { border-color: var(--am-border-strong); box-shadow: 0 4px 16px rgba(0,0,0,.25); }
.am-provider-card.is-active {
  border-color: var(--am-primary);
  box-shadow: 0 0 0 3px var(--am-primary-soft);
}
.am-provider-top { display: flex; align-items: center; gap: 8px; }
.am-provider-name { font-size: 15px; font-weight: 600; }
.am-provider-url { margin-top: 6px; font-size: 12px; color: var(--am-text-dim); word-break: break-all; }
.am-provider-meta { margin-top: 10px; display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.am-provider-count { margin-left: auto; font-size: 12px; color: var(--am-text-dim); }

.am-model-list { display: flex; flex-direction: column; gap: 10px; }
.am-model-row {
  background: var(--am-bg-elevated);
  border: 1px solid var(--am-border);
  border-radius: var(--am-radius);
  padding: 14px 18px;
  display: flex; align-items: center; gap: 16px;
  transition: border-color .18s ease;
}
.am-model-row:hover { border-color: var(--am-border-strong); }
.am-model-info { flex: 1; min-width: 0; }
.am-model-name { font-size: 14.5px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
.am-model-sub { margin-top: 6px; display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.am-model-dim { font-size: 12px; color: var(--am-text-dim); }
.am-model-actions { display: flex; align-items: center; gap: 4px; flex-shrink: 0; }
.am-model-actions .el-button + .el-button { margin-left: 4px; }

.am-empty { color: var(--am-text-dim); font-size: 13px; text-align: center; padding: 28px 0; }

/* pill */
.am-pill {
  display: inline-flex; align-items: center;
  padding: 2px 10px; border-radius: 999px;
  font-size: 11.5px; line-height: 1.6;
  background: var(--am-bg-inset); color: var(--am-text-dim);
  border: 1px solid transparent;
}
.am-pill-current { background: var(--am-primary-soft); color: var(--am-primary); border-color: transparent; }
.am-pill-accent { background: var(--am-primary-soft); color: var(--am-primary); }

/* 图标按钮 */
.am-icon-btn { color: var(--am-text-dim); font-size: 15px; }
.am-icon-btn:hover { color: var(--am-text); }
.am-icon-danger:hover { color: var(--am-danger); }

/* 对话框 */
.am-dialog-desc { margin: -6px 0 16px; font-size: 12.5px; color: var(--am-text-dim); line-height: 1.6; }
.am-form :deep(.el-form-item__label) { font-weight: 500; color: var(--am-text); }
.am-form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px; }
.am-form-divider {
  margin: 4px 0 14px; padding-top: 14px;
  border-top: 1px solid var(--am-border);
  font-size: 13px; font-weight: 600; color: var(--am-text);
}
.am-field-hint {
  display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
  margin: -6px 0 12px; font-size: 12px; color: var(--am-text-dim); line-height: 1.6;
}
.am-field-hint-spec { display: inline-flex; align-items: center; gap: 4px; }
.am-hint-link { color: var(--am-primary); cursor: help; }
.am-opt { display: flex; align-items: center; justify-content: space-between; gap: 16px; width: 100%; }
.am-opt-tokens { font-size: 11.5px; color: var(--am-text-dim); }

.am-collapse { border: 1px solid var(--am-border); border-radius: 12px; padding: 0 16px; margin-top: 6px;
  --el-collapse-border-color: var(--am-border); --el-collapse-header-bg-color: transparent;
  --el-collapse-content-bg-color: transparent; }
.am-collapse :deep(.el-collapse-item__header) { background: transparent; }
.am-collapse-title { display: flex; flex-direction: column; gap: 2px; }
.am-collapse-title span:first-child { font-size: 13.5px; font-weight: 600; }
.am-collapse-desc { font-size: 12px; font-weight: 400; color: var(--am-text-dim); }

.am-alert { border-radius: 10px; margin-bottom: 12px; }

@media (max-width: 768px) {
  .am-page-model { padding: 16px 12px 32px; }
  .am-form-grid { grid-template-columns: 1fr; }
  .am-model-row { flex-direction: column; align-items: flex-start; }
  .am-model-actions { align-self: flex-end; }
}
</style>
