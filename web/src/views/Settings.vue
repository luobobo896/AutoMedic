<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar"><span style="font-weight:600">dsh 运行参数</span></div>
      <el-form :model="dsh" label-width="160px">
        <el-form-item label="dsh 可执行文件">
          <el-input v-model="dsh.bin" placeholder="/usr/local/bin/dsh" />
        </el-form-item>
        <el-form-item label="DSH_HOME">
          <el-input v-model="dsh.home" placeholder="留空继承进程环境" />
        </el-form-item>
        <el-form-item label="权限模式">
          <el-radio-group v-model="dsh.permission_mode">
            <el-radio value="workspace-write">workspace-write（仅工作区可写）</el-radio>
            <el-radio value="danger-full-access">danger-full-access</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="单次修复超时">
          <el-input-number v-model="dsh.timeout_sec" :min="60" :step="60" /> 秒
        </el-form-item>
        <el-form-item label="使用 shell 执行">
          <el-switch v-model="dsh.use_shell" />
          <span class="am-text-dim" style="margin-left:8px;font-size:12px">模板含管道/引号时必须开启</span>
        </el-form-item>
        <el-form-item label="命令模板">
          <el-input v-model="dsh.command_template" type="textarea" :rows="2" />
          <div class="am-text-dim" style="font-size:12px">
            占位符：
            <code v-pre>{{.Bin}}</code>
            <code v-pre>{{.Profile}}</code>
            <code v-pre>{{.Patches}}</code>
            <code v-pre>{{.TaskFile}}</code>
          </div>
        </el-form-item>
        <el-form-item label="模型注入模板">
          <el-input v-model="dsh.patch_template" type="textarea" :rows="8" />
          <div class="am-text-dim" style="font-size:12px">
            占位符：
            <code v-pre>{{.Provider}}</code>
            <code v-pre>{{.Model}}</code>
            <code v-pre>{{.InputContext}}</code>
            <code v-pre>{{.OutputContext}}</code>
            <code v-pre>{{.ExtraYAML}}</code>
          </div>
        </el-form-item>
        <el-form-item label="额外环境变量">
          <el-input v-model="envText" type="textarea" :rows="3" placeholder="每行一个 KEY=VALUE，如 PATH=..." />
        </el-form-item>
      </el-form>
    </div>

    <div class="am-card">
      <div class="am-toolbar"><span style="font-weight:600">Open Code Review</span></div>
      <el-form :model="ocr" label-width="160px">
        <el-form-item label="ocr 可执行文件">
          <el-input v-model="ocr.bin" placeholder="ocr 或绝对路径" />
        </el-form-item>
        <el-form-item label="单次审查超时">
          <el-input-number v-model="ocr.timeout_sec" :min="60" :step="60" /> 秒
        </el-form-item>
        <el-form-item label="审查模型">
          <div class="ocr-model-field">
            <el-radio-group v-model="ocrUseDefault">
              <el-radio :value="true">使用默认模型</el-radio>
              <el-radio :value="false">指定模型</el-radio>
            </el-radio-group>
            <p class="ocr-model-hint">
              默认顺序：仓库审查模型 → 项目审查模型 → 修复模型 → 全局默认。密钥来自大模型配置中心。
            </p>
          </div>
        </el-form-item>
        <el-form-item v-if="!ocrUseDefault" label="指定模型">
          <div class="ocr-model-field">
            <el-select v-model="ocr.model_id" filterable style="width:360px" placeholder="选择大模型配置中心里的模型">
              <el-option v-for="m in models" :key="m.id" :label="modelLabel(m)" :value="m.id" />
            </el-select>
            <p v-if="!models.length" class="ocr-model-hint">暂无可用模型，请先在「大模型配置中心」添加。</p>
          </div>
        </el-form-item>
      </el-form>
    </div>

    <div class="am-card">
      <div class="am-toolbar"><span style="font-weight:600">Git 与发布</span></div>
      <el-form :model="git" label-width="160px">
        <el-form-item label="隔离工作区根目录"><el-input v-model="git.workspace_root" /></el-form-item>
        <el-form-item label="克隆深度">
          <el-input-number v-model="git.depth" :min="0" />
          <span class="am-text-dim" style="margin-left:8px;font-size:12px">0 为全量克隆</span>
        </el-form-item>
        <el-form-item label="复用工作区"><el-switch v-model="git.reuse_workspace" /></el-form-item>
        <el-form-item label="修复分支前缀"><el-input v-model="git.branch_prefix" /></el-form-item>
        <el-form-item label="提交作者">
          <el-input v-model="git.author_name" style="width:200px" />
          <el-input v-model="git.author_email" style="width:240px;margin-left:8px" />
        </el-form-item>
        <el-form-item label="自动推送"><el-switch v-model="git.auto_push" /></el-form-item>
        <el-form-item label="发布钩子">
          <el-input v-model="git.release_hook" placeholder="推送成功后在仓库目录执行的命令，如 ./deploy.sh" />
          <div class="am-text-dim" style="font-size:12px;margin-top:4px">
            对应配置项 `git.release_hook`。推送成功后在隔离工作区执行；项目详情里填写的发布钩子会覆盖此项。留空则不执行。
          </div>
        </el-form-item>
        <el-form-item label="工作区保留天数">
          <el-input-number v-model="git.keep_days" :min="0" /> 天
        </el-form-item>
      </el-form>
      <div>
        <el-button type="primary" @click="save">保存</el-button>
        <el-button @click="load">重新读取</el-button>
        <span class="am-text-dim" style="margin-left:12px;font-size:12px">
          修改即时生效（内存中）；重启后回退为配置文件中的值，建议同步修改 configs/config.yaml
        </span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getSettings, updateSettings, listModels } from '@/api'
import { ElMessage } from 'element-plus'

const dsh = ref({})
const git = ref({})
const ocr = ref({})
const envText = ref('')
const ocrUseDefault = ref(true)
const models = ref([])

function modelLabel(m) {
  const p = m.provider?.name || m.provider_name || ''
  return p ? `${p} / ${m.name}` : (m.name || m.slug || `#${m.id}`)
}

async function load() {
  const [r, m] = await Promise.all([getSettings(), listModels()])
  dsh.value = r.data?.dsh || {}
  git.value = r.data?.git || {}
  ocr.value = r.data?.ocr || {}
  envText.value = (dsh.value.env || []).join('\n')
  models.value = Array.isArray(m.data) ? m.data : (m.data?.list || [])
  ocrUseDefault.value = ocr.value.use_default !== false && !ocr.value.model_id
}

async function save() {
  if (!ocrUseDefault.value && !ocr.value.model_id) {
    ElMessage.error('请选择审查模型，或改回「使用默认模型」')
    return
  }
  await updateSettings({
    dsh: {
      bin: dsh.value.bin,
      home: dsh.value.home,
      timeout_sec: dsh.value.timeout_sec,
      permission_mode: dsh.value.permission_mode,
      use_shell: dsh.value.use_shell,
      command_template: dsh.value.command_template,
      patch_template: dsh.value.patch_template,
      env: envText.value.split('\n').map(s => s.trim()).filter(Boolean)
    },
    ocr: {
      bin: ocr.value.bin,
      timeout_sec: ocr.value.timeout_sec,
      use_default: ocrUseDefault.value,
      model_id: ocrUseDefault.value ? 0 : (ocr.value.model_id || 0)
    },
    git: {
      workspace_root: git.value.workspace_root,
      depth: git.value.depth,
      reuse_workspace: git.value.reuse_workspace,
      branch_prefix: git.value.branch_prefix,
      author_name: git.value.author_name,
      author_email: git.value.author_email,
      auto_push: git.value.auto_push,
      release_hook: git.value.release_hook,
      keep_days: git.value.keep_days
    }
  })
  ElMessage.success('已保存并生效')
  load()
}

onMounted(load)
</script>

<style scoped>
.ocr-model-field {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
  width: 100%;
}
.ocr-model-field :deep(.el-radio-group) {
  display: inline-flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
  min-height: 32px;
}
.ocr-model-field :deep(.el-radio) {
  margin-right: 0;
  height: 32px;
  align-items: center;
}
.ocr-model-field :deep(.el-radio__label) {
  line-height: 32px;
  padding-left: 8px;
}
.ocr-model-hint {
  margin: 0;
  max-width: 36em;
  color: var(--am-text-dim);
  font-size: 12px;
  line-height: 1.6;
}
</style>
