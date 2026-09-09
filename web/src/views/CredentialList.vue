<template>
  <div class="am-page">
    <div class="am-card">
      <div class="am-toolbar">
        <el-button type="primary" :icon="'Plus'" @click="openCreate">新增凭证</el-button>
        <span class="am-text-dim" style="font-size:12px">
          凭证加密存储（AES-256-GCM），可被多个项目 / 仓库复用
        </span>
        <div class="am-flex-1" />
        <el-button :icon="'Refresh'" @click="load" />
      </div>

      <el-table :data="list" v-loading="loading" size="small">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="名称" width="170" />
        <el-table-column label="类型" width="110">
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ TYPE_LABEL[row.type] || row.type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="username" label="账号" width="140" />
        <el-table-column label="密钥" width="200">
          <template #default="{ row }"><span class="am-mono">{{ row.secret_masked }}</span></template>
        </el-table-column>
        <el-table-column label="被引用" min-width="180">
          <template #default="{ row }">
            <template v-if="row.used_by?.length">
              <el-tag v-for="u in row.used_by" :key="u" size="small" style="margin-right:4px">{{ u }}</el-tag>
            </template>
            <span v-else class="am-text-dim">未使用</span>
          </template>
        </el-table-column>
        <el-table-column label="使用次数" width="100">
          <template #default="{ row }">
            <el-button link type="primary" @click="openUsages(row)">{{ row.use_count }}</el-button>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" size="small" @change="v => toggle(row, v)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="dialog" :title="form.id ? '编辑凭证' : '新增凭证'" width="640">
      <el-form :model="form" label-width="110px">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="如：gitlab-ssh-prod" /></el-form-item>
        <el-form-item label="类型">
          <el-select v-model="form.type" style="width:100%">
            <el-option label="SSH 私钥" value="ssh_key" />
            <el-option label="HTTPS 账号密码" value="http_auth" />
            <el-option label="HTTPS Token" value="http_token" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.type !== 'ssh_key'" label="账号">
          <el-input v-model="form.username" placeholder="Token 类型可留空，默认 oauth2" />
        </el-form-item>
        <el-form-item label="密钥内容">
          <el-input v-model="form.secret" type="textarea" :rows="6"
            :placeholder="form.type === 'ssh_key' ? '粘贴完整私钥内容（含 BEGIN/END PRIVATE KEY）' : '密码或 Access Token'" />
        </el-form-item>
        <el-form-item v-if="form.type === 'ssh_key'" label="私钥口令">
          <el-input v-model="form.passphrase" type="password" show-password placeholder="无口令可留空" />
        </el-form-item>
        <el-form-item label="说明"><el-input v-model="form.description" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" @click="submit">保存</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="usageDrawer" title="凭证使用记录" size="45%">
      <el-table :data="usages" size="small">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="action" label="动作" width="100" />
        <el-table-column label="结果" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.result === 'ok' ? 'success' : 'danger'">{{ row.result }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="message" label="说明" min-width="200" show-overflow-tooltip />
        <el-table-column label="时间" width="170">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
      </el-table>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { listCredentials, createCredential, updateCredential, deleteCredential, listCredentialUsages } from '@/api'
import { formatTime } from '@/utils/format'
import { ElMessage, ElMessageBox } from 'element-plus'

const TYPE_LABEL = { ssh_key: 'SSH 私钥', http_auth: '账号密码', http_token: 'Token' }

const list = ref([])
const usages = ref([])
const loading = ref(false)
const dialog = ref(false)
const usageDrawer = ref(false)
const form = ref({ type: 'ssh_key', enabled: true })

async function load() {
  loading.value = true
  try {
    const r = await listCredentials()
    list.value = r.data || []
  } finally { loading.value = false }
}

function openCreate() { form.value = { type: 'ssh_key', enabled: true }; dialog.value = true }
function openEdit(row) {
  form.value = {
    id: row.id, name: row.name, type: row.type, username: row.username,
    description: row.description, enabled: row.enabled, secret: '', passphrase: ''
  }
  dialog.value = true
}

async function submit() {
  const payload = {
    name: form.value.name, type: form.value.type, username: form.value.username || '',
    description: form.value.description || '', enabled: form.value.enabled !== false,
    secret: form.value.secret || '', passphrase: form.value.passphrase || ''
  }
  if (form.value.id) await updateCredential(form.value.id, payload)
  else await createCredential(payload)
  ElMessage.success('已保存')
  dialog.value = false
  load()
}

async function toggle(row, v) {
  await updateCredential(row.id, { enabled: v })
  ElMessage.success('已更新')
}

async function openUsages(row) {
  const r = await listCredentialUsages({ credential_id: row.id, page_size: 100 })
  usages.value = r.data?.list || []
  usageDrawer.value = true
}

async function remove(row) {
  const ok = await ElMessageBox.confirm(`确认删除凭证「${row.name}」？`, '警告', { type: 'warning' }).catch(() => false)
  if (!ok) return
  await deleteCredential(row.id)
  ElMessage.success('已删除')
  load()
}

onMounted(load)
</script>
