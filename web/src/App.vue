<template>
  <div class="am-root">
    <router-view v-if="isPublic" />
    <AppLayout v-else />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from '@/layout/AppLayout.vue'

const route = useRoute()
const isPublic = computed(() => !!route.meta?.public)
</script>

<style>
.am-root { height: 100%; background: var(--am-bg, #161618); }
</style>
