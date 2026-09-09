<template>
  <el-button
    :type="epType"
    :size="epSize"
    :link="type === 'ghost'"
    :loading="loading"
    :disabled="disabled"
    :class="['am-btn', `am-btn--${type}`]"
    :style="block ? { width: '100%' } : undefined"
    :title="label && label.length > 12 ? label : undefined"
  >
    <span v-if="label" class="am-btn__label">{{ label }}</span>
    <slot />
  </el-button>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  // 按钮文字；超过 12 字自动带 title 供复制，配合 CSS 省略号
  label: { type: String, default: '' },
  type: {
    type: String,
    default: 'secondary',
    validator: (v) => ['primary', 'secondary', 'ghost', 'danger'].includes(v),
  },
  size: {
    type: String,
    default: 'medium',
    validator: (v) => ['small', 'medium', 'large'].includes(v),
  },
  loading: Boolean,
  disabled: Boolean,
  block: Boolean,
})

// 语义映射：primary/danger 走 EP type；secondary 用默认样式；ghost 用 link 型
const epType = computed(() => (props.type === 'primary' || props.type === 'danger' ? props.type : undefined))
const epSize = computed(() => (props.size === 'medium' ? undefined : props.size))
</script>

<style scoped>
.am-btn {
  min-width: var(--am-control-min-w);
  padding-inline: var(--am-control-pad-x);
  display: inline-flex;
  align-items: center;
  justify-content: center; /* 文字水平垂直居中 */
  border-radius: var(--am-radius-md);
}
.am-btn--small { min-width: 64px; }
.am-btn--large { min-width: 96px; }
.am-btn__label {
  max-width: 16em;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
@media (max-width: 767.98px) {
  .am-btn { min-height: var(--am-touch); }
}
</style>
