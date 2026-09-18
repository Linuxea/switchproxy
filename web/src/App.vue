<script setup lang="ts">
import { ref } from 'vue'
import { wsState } from './composables/useWs'
import Overview from './views/Overview.vue'
import Connections from './views/Connections.vue'
import Tops from './views/Tops.vue'
import History from './views/History.vue'

const tabs = [
  { key: 'overview', label: '总览' },
  { key: 'connections', label: '连接' },
  { key: 'tops', label: '排行' },
  { key: 'history', label: '历史' },
]
const active = ref('overview')
</script>

<template>
  <div class="app">
    <header class="topbar">
      <div class="brand">
        switchproxy<span class="sub">流量监控</span>
      </div>
      <nav class="tabs">
        <button
          v-for="t in tabs"
          :key="t.key"
          :class="{ active: active === t.key }"
          @click="active = t.key"
        >
          {{ t.label }}
        </button>
      </nav>
      <div class="status" :class="wsState.connected ? 'on' : 'off'">
        <span class="dot"></span>
        {{ wsState.connected ? '实时' : '重连中' }}
      </div>
    </header>
    <main class="content">
      <Overview v-if="active === 'overview'" />
      <Connections v-if="active === 'connections'" />
      <Tops v-if="active === 'tops'" />
      <History v-if="active === 'history'" />
    </main>
  </div>
</template>
