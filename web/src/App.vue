<template>
  <div id="app">
    <!-- 登录等公开页不套后台布局 -->
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
