<template>
  <el-drawer v-model="visible" :title="title" size="72%" destroy-on-close>
    <div class="repo-tree-layout" v-loading="loading">
      <div class="repo-tree-pane">
        <div class="am-text-dim" style="font-size:12px;margin-bottom:8px">
          浅取远端 {{ repo?.branch || 'HEAD' }} 最新提交，用于核对整体代码结构。
        </div>
        <el-tree
          v-if="nodes.length"
          :data="nodes"
          :props="{ label: 'name', children: 'children' }"
          node-key="path"
          highlight-current
          :default-expanded-keys="expanded"
          @node-click="onClick"
        >
          <template #default="{ data }">
            <span class="repo-tree-node">
              <el-icon v-if="data.type === 'dir'"><Folder /></el-icon>
              <el-icon v-else><Document /></el-icon>
              <span>{{ data.name }}</span>
            </span>
          </template>
        </el-tree>
        <div v-else-if="!loading" class="am-text-dim">目录为空或无法读取</div>
        <div v-if="truncated" class="am-text-dim" style="margin-top:8px;font-size:12px">条目过多，已截断显示</div>
      </div>
      <div class="repo-file-pane">
        <div v-if="fileMeta" class="am-toolbar" style="margin-bottom:8px">
          <span class="am-mono">{{ fileMeta.path }}</span>
          <el-tag v-if="fileMeta.binary" size="small" type="warning">二进制</el-tag>
          <el-tag v-if="fileMeta.truncated" size="small">已截断</el-tag>
        </div>
        <div v-if="fileLoading" class="am-text-dim">读取文件…</div>
        <pre v-else-if="fileText" class="am-mono repo-file-pre">{{ fileText }}</pre>
        <div v-else-if="fileMeta?.binary" class="am-text-dim">二进制文件，不展示正文</div>
        <div v-else-if="fileMeta?.truncated" class="am-text-dim">文件过大，已跳过正文</div>
        <div v-else class="am-text-dim">点击文件查看内容（限文本，约 64KB）</div>
      </div>
    </div>
  </el-drawer>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { Folder, Document } from '@element-plus/icons-vue'
import { repoTree, repoFile } from '@/api'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  repo: { type: Object, default: null }
})
const emit = defineEmits(['update:modelValue'])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})
const title = computed(() => (props.repo ? `仓库结构 · ${props.repo.name}` : '仓库结构'))
const loading = ref(false)
const fileLoading = ref(false)
const nodes = ref([])
const expanded = ref([])
const truncated = ref(false)
const fileMeta = ref(null)
const fileText = ref('')

async function load() {
  if (!props.repo?.id) return
  loading.value = true
  nodes.value = []
  expanded.value = []
  truncated.value = false
  fileMeta.value = null
  fileText.value = ''
  try {
    const r = await repoTree(props.repo.id)
    const data = r.data || {}
    nodes.value = data.nodes || []
    truncated.value = !!data.truncated
    expanded.value = nodes.value.filter((n) => n.type === 'dir').map((n) => n.path)
  } finally {
    loading.value = false
  }
}

watch(() => [visible.value, props.repo?.id], () => {
  if (visible.value && props.repo?.id) load()
})

async function onClick(data) {
  if (!data || data.type !== 'file' || !props.repo?.id) return
  fileLoading.value = true
  fileMeta.value = { path: data.path }
  fileText.value = ''
  try {
    const r = await repoFile(props.repo.id, data.path)
    const f = r.data || {}
    fileMeta.value = f
    if (f.binary) fileText.value = ''
    else fileText.value = f.content || ''
  } catch {
    fileText.value = ''
  } finally {
    fileLoading.value = false
  }
}
</script>

<style scoped>
.repo-tree-layout {
  display: flex;
  gap: 16px;
  height: calc(100vh - 160px);
  min-height: 360px;
}
.repo-tree-pane {
  width: 320px;
  flex-shrink: 0;
  overflow: auto;
  border-right: 1px solid var(--am-border);
  padding-right: 12px;
}
.repo-file-pane {
  flex: 1;
  overflow: auto;
  min-width: 0;
}
.repo-tree-node {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.repo-file-pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  line-height: 1.5;
}
:deep(.el-tree) {
  background: transparent;
  color: var(--am-text);
}
</style>
