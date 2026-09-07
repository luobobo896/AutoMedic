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
import { getSettings, updateSettings } from '@/api'
import { ElMessage } from 'element-plus'

const dsh = ref({})
const git = ref({})
const envText = ref('')

async function load() {
  const r = await getSettings()
  dsh.value = r.data?.dsh || {}
  git.value = r.data?.git || {}
  envText.value = (dsh.value.env || []).join('\n')
}

async function save() {
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
