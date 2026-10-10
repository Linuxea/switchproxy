<script setup lang="ts">
import { computed } from 'vue'
import { wsState } from '../composables/useWs'
import { fmtBytes } from '../lib/format'

const domains = computed(() => wsState.snap?.topDomains ?? [])
const rules = computed(() => wsState.snap?.topRules ?? [])

function rows(list: { name: string; up: number; down: number }[]) {
  const totals = list.map((d) => d.up + d.down)
  const max = Math.max(1, ...totals)
  return list.map((d, i) => ({ ...d, total: totals[i], pct: (totals[i] / max) * 100 }))
}
</script>

<template>
  <div class="grid-2">
    <div class="panel">
      <h3 class="panel-title">域名流量 TOP {{ domains.length }} · 本次运行累计</h3>
      <div v-if="domains.length" class="bars">
        <div v-for="d in rows(domains)" :key="d.name" class="bar-row">
          <span class="bar-label" :title="d.name">{{ d.name }}</span>
          <div class="bar-track"><div class="bar-fill green" :style="{ width: `${d.pct}%` }"></div></div>
          <span class="bar-val">{{ fmtBytes(d.total) }}</span>
        </div>
      </div>
      <div v-else class="empty">暂无数据</div>
    </div>

    <div class="panel">
      <h3 class="panel-title">规则流量 TOP {{ rules.length }} · 本次运行累计</h3>
      <div v-if="rules.length" class="bars">
        <div v-for="d in rows(rules)" :key="d.name" class="bar-row">
          <span class="bar-label" :title="d.name">{{ d.name }}</span>
          <div class="bar-track"><div class="bar-fill amber" :style="{ width: `${d.pct}%` }"></div></div>
          <span class="bar-val">{{ fmtBytes(d.total) }}</span>
        </div>
      </div>
      <div v-else class="empty">暂无数据</div>
    </div>
  </div>
</template>

<style scoped>
.bars {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
</style>
