<script setup lang="ts">
import { computed, ref } from 'vue'
import { wsState } from './composables/useWs'
import { themeState, toggleTheme } from './composables/useTheme'
import Overview from './views/Overview.vue'
import Connections from './views/Connections.vue'
import Tops from './views/Tops.vue'
import History from './views/History.vue'
import Settings from './views/Settings.vue'

const tabs = [
  { key: 'overview', label: '总览' },
  { key: 'connections', label: '连接' },
  { key: 'tops', label: '排行' },
  { key: 'history', label: '历史' },
  { key: 'settings', label: '设置' },
]
const active = ref('overview')
// 仅图表视图需要随主题重挂载（重建 ECharts 调色板）；
// Settings/Connections 不带 :key，切主题不丢未保存的编辑/筛选状态。
const themeKey = computed(() => (themeState.dark ? 'dark' : 'light'))
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
      <button
        class="theme-toggle"
        :title="themeState.dark ? '切换到亮色' : '切换到暗色'"
        @click="toggleTheme"
      >
        {{ themeState.dark ? '☀' : '🌙' }}
      </button>
    </header>
    <!-- 图表视图带 :key 随主题重挂载重绘；有本地状态的视图不带，避免切主题丢编辑 -->
    <main class="content">
      <Overview v-if="active === 'overview'" :key="themeKey" />
      <Connections v-if="active === 'connections'" />
      <Tops v-if="active === 'tops'" :key="themeKey" />
      <History v-if="active === 'history'" :key="themeKey" />
      <Settings v-if="active === 'settings'" />
    </main>
  </div>
</template>
