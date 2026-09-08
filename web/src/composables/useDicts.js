import { reactive, ref } from 'vue'
import { listDicts } from '@/api'

const grouped = ref({})
const groups = ref([])
const loaded = ref(false)
let pending = null

function parseExtra(raw) {
  if (!raw || typeof raw === 'object') return raw || {}
  try { return JSON.parse(raw) } catch { return {} }
}

export function useDicts() {
  async function load(force) {
    if (loaded.value && !force) return grouped.value
    if (pending && !force) return pending
    pending = (async () => {
      const r = await listDicts({ all: 1 })
      const list = r.data?.list || []
      groups.value = r.data?.groups || []
      const m = {}
      for (const it of list) {
        const g = it.group
        if (!m[g]) m[g] = []
        m[g].push({ ...it, extra: parseExtra(it.extra) })
      }
      grouped.value = m
      loaded.value = true
      pending = null
      return m
    })()
    return pending
  }

  function items(group, { enabledOnly = true } = {}) {
    const list = grouped.value[group] || []
    return enabledOnly ? list.filter(x => x.enabled !== false) : list
  }

  function options(group) {
    return items(group).map(x => ({ value: x.value, label: x.label || x.value, extra: x.extra }))
  }

  function values(group) {
    return items(group).map(x => x.value)
  }

  function numberOptions(group) {
    return items(group).map(x => ({ value: Number(x.value), label: x.label || x.value }))
  }

  function providerPresets() {
    return items('provider_kind').map(x => ({
      kind: x.value,
      name: x.label || x.value,
      key: x.extra?.key || x.value,
      base_url: x.extra?.base_url || ''
    }))
  }

  function slugsForKind(kind) {
    if (!kind) return items('model_slug').map(x => x.value)
    const parent = items('provider_kind').find(x => x.value === kind)
    return items('model_slug').filter(x => {
      if (parent && x.parent_id && Number(x.parent_id) === Number(parent.id)) return true
      return x.extra?.kind === kind
    }).map(x => x.value)
  }

  function childrenOf(id) {
    if (!id) return []
    const pid = Number(id)
    const list = []
    for (const g of Object.keys(grouped.value || {})) {
      for (const it of grouped.value[g] || []) {
        if (Number(it.parent_id) === pid) list.push(it)
      }
    }
    return list
  }

  function temperatureValue(v) {
    if (v === 'default' || v == null) return ''
    return String(v)
  }

  return reactive({ grouped, groups, load, items, options, values, numberOptions, providerPresets, slugsForKind, childrenOf, temperatureValue })
}

export function splitCSV(s) {
  return String(s || '').split(/[,，]/).map(x => x.trim()).filter(Boolean)
}

export function joinCSV(arr) {
  return (arr || []).map(x => String(x).trim()).filter(Boolean).join(',')
}
