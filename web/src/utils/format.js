import { ElMessage } from 'element-plus'

export const STATUS_META = {
  pending: { label: '待处理', type: 'info' },
  running: { label: '处理中', type: 'primary' },
  confirming: { label: '待人工确认', type: 'warning' },
  success: { label: '成功', type: 'success' },
  failed: { label: '失败', type: 'danger' },
  ignored: { label: '已忽略', type: 'info' },
  cancelled: { label: '已取消', type: 'info' },
  rejected: { label: '已驳回', type: 'danger' }
}

export const EVENT_STATUS_META = {
  received: { label: '已接收', type: 'info' },
  matched: { label: '已命中规则', type: 'warning' },
  ignored: { label: '已忽略', type: 'info' },
  dropped: { label: '已丢弃', type: 'info' }
}

export const LEVEL_META = {
  fatal: { label: 'FATAL', type: 'danger' },
  error: { label: 'ERROR', type: 'danger' },
  warn: { label: 'WARN', type: 'warning' },
  info: { label: 'INFO', type: 'info' }
}

export const STAGE_LABEL = {
  pending: '排队中',
  prepare: '准备工作区',
  dsh: 'dsh 修复中',
  commit: '提交代码',
  push: '推送远端',
  release: '发布中',
  confirming: '待人工确认',
  done: '完成',
  ignored: '已忽略',
  rejected: '已驳回',
  cancelled: '已取消'
}

export function formatDuration(ms) {
  if (!ms) return '-'
  const s = Math.round(ms / 1000)
  if (s < 60) return s + 's'
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m${s % 60}s`
  const h = Math.floor(m / 60)
  return `${h}h${m % 60}m`
}

export function formatTime(t) {
  if (!t) return '-'
  const d = new Date(t)
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function formatTokens(n) {
  if (!n) return '-'
  if (n >= 1000000) return (n / 1000000).toFixed(n % 1000000 === 0 ? 0 : 1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(n % 1000 === 0 ? 0 : 0) + 'K'
  return String(n)
}

export function parseJSON(v, fallback) {
  if (!v) return fallback
  if (typeof v === 'object') return v
  try {
    return JSON.parse(v)
  } catch {
    return fallback
  }
}

export function copyText(text) {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(text).then(() => ElMessage.success('已复制'))
    return
  }
  const ta = document.createElement('textarea')
  ta.value = text
  document.body.appendChild(ta)
  ta.select()
  document.execCommand('copy')
  document.body.removeChild(ta)
  ElMessage.success('已复制')
}

// 简易 ANSI 清理：终端以纯文本渲染
export function stripANSI(s) {
  // eslint-disable-next-line no-control-regex
  return s.replace(/\x1B\[[0-9;]*[A-Za-z]/g, '').replace(/\x1B\][^\x07]*\x07/g, '')
}
