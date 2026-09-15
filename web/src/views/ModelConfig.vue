<template>
  <div class="am-page am-page-model">
    <div class="am-page-head">
      <div>
        <div class="am-page-head__crumb">首页</div>
        <h1 class="am-page-head__title">大模型配置</h1>
        <p class="am-page-head__desc">
          厂家只配接入信息，模型逐个设置输入/输出上下文；下拉选项来自「选项字典」。
        </p>
      </div>
      <div class="am-page-head__actions">
        <el-button :icon="'Refresh'" aria-label="刷新" @click="load" />
        <el-button :icon="'Plus'" @click="openProvider">新增厂家</el-button>
        <el-button type="primary" :icon="'Plus'" :disabled="!providers.length" @click="openModel">新增模型</el-button>
      </div>
    </div>

    <div class="am-seg-tabs" role="tablist" aria-label="大模型配置视图">
      <button
        v-for="t in tabs"
        :key="t.key"
        type="button"
        role="tab"
        class="am-seg-tabs__item"
        :class="{ 'is-current': tab === t.key }"
        :aria-selected="tab === t.key"
        @click="tab = t.key"
      >{{ t.label }}</button>
    </div>

    <!-- 模型列表 -->
    <div v-show="tab === 'models'" class="am-panel">
      <div class="am-panel__body">
        <div class="am-filterbar">
          <el-input
            v-model="modelKeyword"
            class="am-filterbar__search"
            placeholder="搜索模型名称、标识或厂家"
            clearable
            :prefix-icon="'Search'"
          />
          <el-select v-model="vendorFilter" clearable placeholder="全部厂家" style="width: 180px">
            <el-option v-for="p in providers" :key="p.id" :label="p.name" :value="p.id" />
          </el-select>
          <button
            v-for="s in statusFilters"
            :key="s.key"
            type="button"
            class="am-chip-btn"
            :class="{ 'is-on': statusFilter === s.key }"
            @click="statusFilter = s.key"
          >{{ s.label }}<span class="am-chip-btn__n">{{ statusCount(s.key) }}</span></button>
          <button type="button" class="am-chip-btn" :class="{ 'is-on': onlyDefault }" @click="onlyDefault = !onlyDefault">
            仅默认
          </button>
          <span class="am-filterbar__end">共 {{ filteredModels.length }} 个模型</span>
        </div>

        <el-table :data="filteredModels" v-loading="booting" size="small" row-key="id">
          <el-table-column label="模型" min-width="240">
            <template #default="{ row }">
              <div class="am-cell-title">
                {{ row.name }}
                <el-tag v-if="row.is_default" type="primary" size="small">默认</el-tag>
              </div>
              <div class="am-cell-sub am-mono">{{ row.slug }}</div>
            </template>
          </el-table-column>
          <el-table-column label="厂家" width="170">
            <template #default="{ row }">
              <div>{{ providerName(row.provider_id) }}</div>
              <div class="am-cell-sub">{{ providerKind(row.provider_id) || 'custom' }}</div>
            </template>
          </el-table-column>
          <el-table-column label="输入上下文" width="120">
            <template #default="{ row }">{{ formatTokens(row.input_context) }}</template>
          </el-table-column>
          <el-table-column label="输出长度" width="120">
            <template #default="{ row }">{{ formatTokens(row.output_context) }}</template>
          </el-table-column>
          <el-table-column label="最大轮次" width="100">
            <template #default="{ row }">{{ row.max_turns }}</template>
          </el-table-column>
          <el-table-column label="温度" width="90">
            <template #default="{ row }">{{ row.temperature || '默认' }}</template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-switch v-model="row.enabled" size="small" :disabled="saving" @change="v => toggleModel(row, v)" />
            </template>
          </el-table-column>
          <el-table-column label="操作" width="196">
            <template #default="{ row }">
              <el-button v-if="!row.is_default" link type="primary" :disabled="saving" @click="setDefault(row)">设为默认</el-button>
              <el-button link type="primary" :disabled="saving" @click="editModel(row)">编辑</el-button>
              <el-button link type="danger" :disabled="saving" @click="removeModel(row)">删除</el-button>
            </template>
          </el-table-column>
          <template #empty>
            <div class="am-empty am-empty--table">
              <div class="am-empty__title">未找到模型</div>
              <div class="am-empty__desc">
                {{ providers.length ? '换个关键字或清空筛选；也可以直接新增一个模型。' : '先在「厂家接入」里添加一个厂家，再为它配置模型。' }}
              </div>
              <div class="am-empty__actions">
                <el-button size="small" @click="tab = 'vendors'; providers.length ? null : openProvider()">
                  {{ providers.length ? '去厂家接入' : '新增厂家' }}
                </el-button>
              </div>
            </div>
          </template>
        </el-table>
      </div>
    </div>

    <!-- 厂家接入 -->
    <div v-show="tab === 'vendors'" class="am-panel">
      <div class="am-panel__head">
        <div>
          <h2 class="am-panel__title">厂家接入</h2>
          <p class="am-panel__desc">
            类型取自「选项字典 · 厂家类型」，选中类型自动带出标识与 Base URL；点击一行即切换当前使用的厂家。
          </p>
        </div>
        <div class="am-panel__actions">
          <router-link class="am-link" :to="{ path: '/dicts', query: { group: 'provider_kind' } }">维护厂家类型</router-link>
          <el-button size="small" :icon="'Plus'" @click="openProvider">新增厂家</el-button>
        </div>
      </div>
      <div class="am-panel__body">
        <el-table :data="providers" v-loading="booting" size="small" :row-class-name="providerRowClass" @row-click="onSelectProvider">
          <el-table-column label="厂家" min-width="200">
            <template #default="{ row }">
              <div class="am-cell-title">
                {{ row.name }}
                <el-tag v-if="current?.id === row.id" type="primary" size="small">当前</el-tag>
              </div>
              <div class="am-cell-sub am-mono">{{ row.key }}</div>
            </template>
          </el-table-column>
          <el-table-column label="类型" width="160">
            <template #default="{ row }">
              <el-tag v-if="dictHasKind(row.kind)" size="small" effect="plain" :title="`字典显示名：${dictKindLabel(row.kind)}`">
                <span class="am-mono">{{ row.kind }}</span>
              </el-tag>
              <el-tag v-else size="small" type="warning">字典缺失：{{ row.kind || '未填' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Base URL" min-width="220" show-overflow-tooltip>
            <template #default="{ row }"><span class="am-mono">{{ row.base_url || '-' }}</span></template>
          </el-table-column>
          <el-table-column label="模型数" width="90">
            <template #default="{ row }">{{ providerModelCount(row.id) }}</template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '停用' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="130">
            <template #default="{ row }">
              <el-button link type="primary" :disabled="saving" @click.stop="editProvider(row)">编辑</el-button>
              <el-button link type="danger" :disabled="saving" @click.stop="removeProvider(row)">删除</el-button>
            </template>
          </el-table-column>
          <template #empty>
            <div class="am-empty am-empty--table">
              <div class="am-empty__title">还没有厂家</div>
              <div class="am-empty__desc">厂家的「类型」来自选项字典，新增后即可为它配置模型。</div>
              <div class="am-empty__actions">
                <el-button size="small" type="primary" @click="openProvider">新增厂家</el-button>
              </div>
            </div>
          </template>
        </el-table>
      </div>
      <div class="am-panel__foot">
        <div class="am-panel__stat">
          <span>当前使用：<b>{{ current?.name || '未选择' }}</b></span>
          <span>默认模型：<b>{{ defaultModel?.name || '未设置' }}</b></span>
        </div>
      </div>
    </div>

    <!-- dsh 调用预览 -->
    <div v-show="tab === 'preview'" class="am-panel">
      <div class="am-panel__head">
        <div>
          <h2 class="am-panel__title">dsh 调用预览</h2>
          <p class="am-panel__desc">按当前厂家与默认模型生成的 cordis patch 层、实际命令与环境变量。</p>
        </div>
        <div class="am-panel__actions">
          <span class="am-text-dim am-hint">
            {{ current?.name || '未选厂家' }} · {{ defaultModel?.slug || '未选模型' }}
          </span>
        </div>
      </div>
      <div class="am-panel__body">
        <div class="am-diff" v-if="current && defaultModel">
          <div class="hunk"># 系统生成的 cordis patch 层（--patch 注入）</div>
          <div v-for="(l, i) in patchPreview" :key="i">{{ l }}</div>
          <div class="hunk" style="margin-top:8px"># 实际调用命令</div>
          <div>dsh --profile headless --patch /tmp/am-dsh-xxx/model.patch.yml "$(cat /tmp/am-dsh-xxx/task.txt)"</div>
          <div class="hunk" style="margin-top:8px"># 环境变量</div>
          <div>DSH_PERMISSION_MODE={{ settings.dsh?.permission_mode || 'workspace-write' }}</div>
          <div>{{ envKeyPreview }}=******（未配置则沿用 dsh 自身凭证）</div>
        </div>
        <div v-else class="am-empty am-empty--table">
          <div class="am-empty__title">还没有可预览的配置</div>
          <div class="am-empty__desc">先在「厂家接入」选择厂家，并在「模型列表」里启用一个模型。</div>
          <div class="am-empty__actions">
            <el-button size="small" @click="tab = 'vendors'">去厂家接入</el-button>
          </div>
        </div>
      </div>
    </div>

    <!-- 厂家对话框 -->
    <el-dialog v-model="providerDialog" :title="providerForm.id ? '编辑厂家' : '新增厂家'" width="620" class="am-dialog">
      <p class="am-dialog-desc">厂家对接 OpenAI 兼容 API，API Key 加密存储；「类型」引用选项字典的厂家类型。</p>
      <div class="am-form-stack">
        <div class="am-form-cols">
          <div class="am-field">
            <label class="am-field__label" for="pv-kind">类型</label>
            <el-select id="pv-kind" v-model="providerForm.kind" style="width:100%" @change="onProviderKind">
              <el-option v-for="p in dict.providerPresets()" :key="p.kind" :label="p.name" :value="p.kind" />
            </el-select>
            <div class="am-field__origin">
              来自选项字典 · 厂家类型（{{ dictProviderKinds.length }} 项）
              <router-link class="am-link" :to="{ path: '/dicts', query: { group: 'provider_kind' } }">去维护</router-link>
            </div>
          </div>
          <div class="am-field">
            <label class="am-field__label" for="pv-name">厂家名称</label>
            <el-input id="pv-name" v-model="providerForm.name" placeholder="展示名称" />
            <div class="am-field__help">列表与统计里显示的厂家名。</div>
          </div>
        </div>
        <div class="am-field">
          <label class="am-field__label" for="pv-key">标识 key</label>
          <el-input id="pv-key" v-model="providerForm.key" placeholder="传给 dsh 的 provider 标识" />
          <div class="am-field__help">dsh 侧识别厂家用的标识，需唯一；选择类型后会自动填充。</div>
        </div>
        <div class="am-field">
          <label class="am-field__label" for="pv-url">Base URL</label>
          <el-input id="pv-url" v-model="providerForm.base_url" placeholder="选类型后自动填，自定义可改" />
          <div class="am-field__help">OpenAI 兼容端点根地址，例如 https://api.deepseek.com/v1。</div>
        </div>
        <div class="am-field">
          <label class="am-field__label" for="pv-key-secret">API Key</label>
          <el-input id="pv-key-secret" v-model="providerForm.api_key" type="password" show-password
            placeholder="留空则使用 dsh 自身凭证" />
          <div class="am-field__help">AES-256-GCM 加密存储，保存后不回显。</div>
        </div>
        <div class="am-form-cols">
          <div class="am-field">
            <label class="am-field__label" for="pv-remark">备注</label>
            <el-input id="pv-remark" v-model="providerForm.remark" placeholder="可选" />
          </div>
          <div class="am-field">
            <span class="am-field__label">启用</span>
            <el-switch v-model="providerForm.enabled" />
            <div class="am-field__help">停用后不会出现在新建模型的厂家下拉里。</div>
          </div>
        </div>
      </div>
      <template #footer>
        <el-button @click="providerDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitProvider">保存厂家</el-button>
      </template>
    </el-dialog>

    <!-- 模型对话框 -->
    <el-dialog v-model="modelDialog" :title="modelForm.id ? '编辑模型' : '新增模型'" width="680" class="am-dialog">
      <p class="am-dialog-desc">输出长度档位按模型官方文档声明过滤；384K 仅对官方支持 384K 输出的模型开放。</p>
      <div class="am-form-stack">
        <div class="am-form-cols">
          <div class="am-field">
            <label class="am-field__label">所属厂家</label>
            <el-select v-model="modelForm.provider_id" style="width:100%">
              <el-option v-for="p in providers" :key="p.id" :label="p.name" :value="p.id" />
            </el-select>
            <div class="am-field__help">决定走哪个厂家的凭证与 Base URL。</div>
          </div>
          <div class="am-field">
            <label class="am-field__label">模型标识 slug</label>
            <el-select v-model="modelForm.slug" filterable allow-create default-first-option style="width:100%"
              placeholder="选择或输入官方模型名" @change="onSlugChange">
              <el-option v-for="s in slugOptions" :key="s" :label="s" :value="s" />
            </el-select>
            <div class="am-field__origin">
              来自选项字典 · 模型标识（{{ slugOptions.length }} 项，父级 = 该厂家类型）
              <router-link class="am-link" :to="{ path: '/dicts', query: { group: 'model_slug' } }">去维护</router-link>
            </div>
          </div>
        </div>
        <div class="am-field">
          <label class="am-field__label">模型名称</label>
          <el-input v-model="modelForm.name" placeholder="展示用名称" />
        </div>

        <div class="am-form-divider">上下文设置</div>
        <div class="am-form-cols">
          <div class="am-field">
            <label class="am-field__label">上下文窗口（输入）</label>
            <el-select v-model="modelForm.input_context" filterable allow-create default-first-option style="width:100%">
              <el-option v-for="o in inputOptions" :key="o.v" :label="o.label" :value="o.v" />
            </el-select>
          </div>
          <div class="am-field">
            <label class="am-field__label">输出长度（最大输出）</label>
            <el-select v-model="modelForm.output_context" filterable allow-create default-first-option style="width:100%">
              <el-option v-for="o in outputOptions" :key="o.v" :label="o.label" :value="o.v">
                <span class="am-opt">{{ o.label }}<span class="am-opt-tokens">{{ o.tokens }} tokens</span></span>
              </el-option>
            </el-select>
          </div>
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
            <div class="am-field">
              <label class="am-field__label">最大推理轮次</label>
              <el-input-number v-model="modelForm.max_turns" :min="1" :max="1000" />
            </div>
            <div class="am-field">
              <label class="am-field__label">温度</label>
              <el-select v-model="modelForm.temperature" style="width:100%">
                <el-option v-for="o in tempOptions" :key="o.value || 'default'" :label="o.label" :value="o.value" />
              </el-select>
              <div class="am-field__origin">
                来自选项字典 · 模型温度（{{ dict.items('temperature').length }} 项）
                <router-link class="am-link" :to="{ path: '/dicts', query: { group: 'temperature' } }">去维护</router-link>
              </div>
            </div>
            <div class="am-field">
              <label class="am-field__label">额外参数</label>
              <el-input v-model="extraText" type="textarea" :rows="3"
                placeholder='JSON，会合并进 dsh patch 配置，如 {"topP": 0.9}' />
            </div>
            <div class="am-form-cols">
              <div class="am-field"><span class="am-field__label">设为默认</span><el-switch v-model="modelForm.is_default" /></div>
              <div class="am-field"><span class="am-field__label">启用</span><el-switch v-model="modelForm.enabled" /></div>
            </div>
          </el-collapse-item>
        </el-collapse>
      </div>
      <template #footer>
        <el-button @click="modelDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitModel">保存模型</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
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
const route = useRoute()
const providers = ref([])
const models = ref([])
const settings = ref({})
const current = ref(null)
const saving = ref(false)
const booting = ref(false)

// 视图分段：模型列表 / 厂家接入 / 调用预览（对齐参考平台的「元信息 | 部署」分段）
const tabs = [
  { key: 'models', label: '模型列表' },
  { key: 'vendors', label: '厂家接入' },
  { key: 'preview', label: '调用预览' }
]
const tab = ref('models')
const modelKeyword = ref('')
const vendorFilter = ref('')
const statusFilter = ref('all')
const onlyDefault = ref(false)
const statusFilters = [
  { key: 'all', label: '全部' },
  { key: 'on', label: '启用' },
  { key: 'off', label: '停用' }
]

const providerName = id => providers.value.find(p => p.id === id)?.name || '-'
const providerKind = id => providers.value.find(p => p.id === id)?.kind || ''
const providerRowClass = ({ row }) => (current.value?.id === row.id ? 'am-row-current' : '')

// 字典是厂家类型/模型标识的唯一事实源：这里只读字典，缺项显式提示而不是静默兜底
const dictProviderKinds = computed(() => dict.items('provider_kind', { enabledOnly: false }))
const dictHasKind = kind => dictProviderKinds.value.some(k => k.value === kind)
const dictKindLabel = kind => dictProviderKinds.value.find(k => k.value === kind)?.label || kind

const statusCount = (key) => {
  if (key === 'on') return models.value.filter(m => m.enabled).length
  if (key === 'off') return models.value.filter(m => !m.enabled).length
  return models.value.length
}

const filteredModels = computed(() => {
  const kw = modelKeyword.value.trim().toLowerCase()
  return models.value.filter((m) => {
    if (vendorFilter.value && m.provider_id !== vendorFilter.value) return false
    if (statusFilter.value === 'on' && !m.enabled) return false
    if (statusFilter.value === 'off' && m.enabled) return false
    if (onlyDefault.value && !m.is_default) return false
    if (!kw) return true
    return [m.name, m.slug, providerName(m.provider_id)].some(v => String(v || '').toLowerCase().includes(kw))
  })
})

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
  booting.value = true
  try {
    const r = await listLLMConfig()
    providers.value = r.data?.providers || []
    models.value = r.data?.models || []
    if (!current.value && providers.value.length) current.value = providers.value[0]
    if (current.value) current.value = providers.value.find(p => p.id === current.value.id) || providers.value[0] || null
  } finally {
    booting.value = false
  }
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
  if (saving.value) return
  saving.value = true
  try {
    const payload = {
      name: providerForm.value.name, key: providerForm.value.key, kind: providerForm.value.kind,
      base_url: providerForm.value.base_url || '', remark: providerForm.value.remark || '',
      enabled: providerForm.value.enabled !== false, api_key: providerForm.value.api_key || ''
    }
    if (providerForm.value.id) await updateProvider(providerForm.value.id, payload)
    else await createProvider(payload)
    ElMessage.success('已保存')
    providerDialog.value = false
    await load()
  } finally { saving.value = false }
}

async function removeProvider(row) {
  if (saving.value) return
  saving.value = true
  try {
    const ok = await ElMessageBox.confirm(`确认删除厂家「${row.name}」？`, '警告', { type: 'warning' }).catch(() => false)
    if (!ok) return
    await deleteProvider(row.id)
    await load()
  } finally { saving.value = false }
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
  if (saving.value) return
  saving.value = true
  try {
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
    await load()
  } finally { saving.value = false }
}

async function toggleModel(row, v) {
  if (saving.value) return
  saving.value = true
  try {
    await updateModel(row.id, { enabled: v })
    ElMessage.success('已更新')
  } finally { saving.value = false }
}
async function setDefault(row) {
  if (saving.value) return
  saving.value = true
  try {
    await updateModel(row.id, { is_default: true })
    ElMessage.success('已设为默认')
    await load()
  } finally { saving.value = false }
}

async function removeModel(row) {
  if (saving.value) return
  saving.value = true
  try {
    const ok = await ElMessageBox.confirm(`确认删除模型「${row.name}」？`, '警告', { type: 'warning' }).catch(() => false)
    if (!ok) return
    await deleteModel(row.id)
    await load()
  } finally { saving.value = false }
}

onMounted(async () => {
  try { settings.value = (await getSettings()).data || {} } catch { /* ignore */ }
  await dict.load()
  await load()
  applyRouteFilter()
})

// 从字典管理页跳进来时带 kind/slug，直接落到对应视图与筛选；
// 同页只改 query 不会重新挂载组件，所以必须 watch
watch(() => [route.query.kind, route.query.slug], applyRouteFilter)

function applyRouteFilter() {
  if (route.query.kind) {
    tab.value = 'vendors'
    current.value = providers.value.find(p => p.kind === route.query.kind) || current.value
  }
  if (route.query.slug) {
    tab.value = 'models'
    modelKeyword.value = String(route.query.slug)
  }
}
</script>

<style scoped>
/* 表格单元格：主标题 + 次行（参考平台的紧凑两行单元格） */
.am-cell-title { display: flex; align-items: center; gap: 8px; font-weight: 600; color: var(--am-text); }
.am-cell-sub { margin-top: 2px; font-size: var(--am-font-xs); color: var(--am-text-dim); }

/* 当前厂家行：淡蓝底 + 左侧色条，和选中态一致 */
:deep(.el-table__row.am-row-current) { background: var(--am-primary-soft); cursor: pointer; }
:deep(.el-table__row.am-row-current td:first-child) { box-shadow: inset 3px 0 0 var(--am-primary); }
:deep(.el-table__row) { cursor: pointer; }

/* 下拉选项：右对齐的 tokens 标注 */
.am-opt { display: flex; align-items: center; justify-content: space-between; gap: 16px; width: 100%; }
.am-opt-tokens { font-size: var(--am-font-xs); color: var(--am-text-dim); }

/* 对话框：说明、分组标题与折叠区 */
.am-dialog-desc { margin: -6px 0 18px; font-size: var(--am-font-sm); color: var(--am-text-dim); line-height: var(--am-leading-relaxed); }
.am-form-divider {
  margin: 6px 0 2px; padding-top: var(--am-space-4);
  border-top: 1px solid var(--am-border-subtle);
  font-size: var(--am-font-sm); font-weight: 600; color: var(--am-text);
}
.am-field-hint {
  display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
  margin: -4px 0 4px; font-size: var(--am-font-xs); color: var(--am-text-dim); line-height: var(--am-leading-relaxed);
}
.am-field-hint-spec { display: inline-flex; align-items: center; gap: 4px; }
.am-hint-link { color: var(--am-primary); cursor: help; }

.am-collapse {
  border: 1px solid var(--am-border);
  border-radius: var(--am-radius-md);
  padding: 0 var(--am-space-4);
  margin-top: var(--am-space-2);
  --el-collapse-border-color: var(--am-border);
  --el-collapse-header-bg-color: transparent;
  --el-collapse-content-bg-color: transparent;
}
.am-collapse :deep(.el-collapse-item__header) { background: transparent; }
.am-collapse :deep(.el-collapse-item__content) { padding-bottom: var(--am-space-4); }
.am-collapse-title { display: flex; flex-direction: column; gap: 2px; }
.am-collapse-title span:first-child { font-size: var(--am-font-md); font-weight: 600; }
.am-collapse-desc { font-size: var(--am-font-xs); font-weight: 400; color: var(--am-text-dim); }

.am-alert { border-radius: var(--am-radius-md); margin-bottom: var(--am-space-3); }

@media (max-width: 767.98px) {
  :deep(.el-table__row) { cursor: default; }
}
</style>
