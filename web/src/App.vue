<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { wsState } from './composables/useWs'
import { themeState, toggleTheme } from './composables/useTheme'
import { fmtUptime } from './lib/format'
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

const ov = computed(() => wsState.snap?.overview ?? null)
const version = ref('')

onMounted(async () => {
  try {
    const res = await fetch('/api/config')
    if (res.ok) version.value = ((await res.json()) as { version?: string }).version ?? ''
  } catch {
    /* 版本号读不到就不显示 */
  }
})
</script>

<template>
  <div class="app">
    <aside class="rail">
      <div class="rail-brand">
        <!-- 道岔印记：一条干线扳出两条轨 -->
        <svg width="22" height="22" viewBox="0 0 22 22" fill="none" aria-hidden="true">
          <path d="M1.5 11 H8 C13 11 13 4.5 20.5 4.5" stroke="var(--ink)" stroke-width="2" />
          <path d="M8 11 C13 11 13 17.5 20.5 17.5" stroke="var(--amber)" stroke-width="2" />
        </svg>
        <span class="brand-name">
          switchproxy
          <span class="brand-sub">流量监控</span>
        </span>
      </div>

      <nav class="rail-nav">
        <button
          v-for="t in tabs"
          :key="t.key"
          :class="{ active: active === t.key }"
          @click="active = t.key"
        >
          {{ t.label }}
          <span v-if="t.key === 'connections' && ov && ov.activeConns > 0" class="nav-badge">
            {{ ov.activeConns }}
          </span>
        </button>
      </nav>

      <div class="rail-plate">
        <div class="plate-row" :class="wsState.connected ? 'on' : 'off'">
          <span class="lamp">
            <span class="dot" :style="{ background: wsState.connected ? 'var(--green)' : 'var(--red)' }"></span>
          </span>
          {{ wsState.connected ? '实时' : '重连中' }}
        </div>
        <div class="plate-row uptime">
          <span v-if="ov">运行 {{ fmtUptime(ov.uptime) }}</span>
        </div>
        <div class="plate-row version" v-if="version">
          <span class="mono">v{{ version }}</span>
        </div>
        <div class="plate-row">
          <span class="plate-spacer"></span>
          <button
            class="icon-btn"
            :title="themeState.dark ? '切换到亮色' : '切换到暗色'"
            :aria-label="themeState.dark ? '切换到亮色' : '切换到暗色'"
            @click="toggleTheme"
          >
            <svg v-if="themeState.dark" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
              <circle cx="12" cy="12" r="4.4" />
              <path d="M12 2.5v2.4M12 19.1v2.4M2.5 12h2.4M19.1 12h2.4M5.2 5.2l1.7 1.7M17.1 17.1l1.7 1.7M18.8 5.2l-1.7 1.7M6.9 17.1l-1.7 1.7" />
            </svg>
            <svg v-else width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M20.6 14.2A8.8 8.8 0 0 1 9.8 3.4a8.8 8.8 0 1 0 10.8 10.8Z" />
            </svg>
          </button>
        </div>
      </div>
    </aside>

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
