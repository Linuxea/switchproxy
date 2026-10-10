<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { wsState } from '../composables/useWs'
import { useChart, axisCommon, chartPalette } from '../lib/chart'
import { fmtBytes } from '../lib/format'

const domEl = ref<HTMLElement | null>(null)
const ruleEl = ref<HTMLElement | null>(null)
const { apply: applyDom } = useChart(domEl)
const { apply: applyRule } = useChart(ruleEl)

const domains = computed(() => wsState.snap?.topDomains ?? [])
const rules = computed(() => wsState.snap?.topRules ?? [])

watch(domains, (list) => {
  const ax = axisCommon()
  const p = chartPalette()
  applyDom({
    animationDurationUpdate: 300,
    grid: { left: 8, right: 24, top: 8, bottom: 4, containLabel: true },
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' }, valueFormatter: (v) => fmtBytes(Number(v)) },
    xAxis: {
      type: 'value',
      ...ax,
      axisLabel: { ...ax.axisLabel, formatter: (v: number) => fmtBytes(v) },
    },
    yAxis: {
      type: 'category',
      data: [...list].reverse().map((d) => d.name),
      ...ax,
      axisLabel: {
        color: p.text,
        width: 180,
        overflow: 'truncate',
        fontFamily: 'JetBrains Mono, Consolas, monospace',
      },
    },
    series: [
      {
        type: 'bar',
        data: [...list].reverse().map((d) => d.up + d.down),
        itemStyle: { color: p.down, borderRadius: [0, 3, 3, 0] },
        barMaxWidth: 14,
      },
    ],
  })
})

watch(rules, (list) => {
  const ax = axisCommon()
  const p = chartPalette()
  applyRule({
    animationDurationUpdate: 300,
    grid: { left: 8, right: 24, top: 8, bottom: 4, containLabel: true },
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' }, valueFormatter: (v) => fmtBytes(Number(v)) },
    xAxis: {
      type: 'value',
      ...ax,
      axisLabel: { ...ax.axisLabel, formatter: (v: number) => fmtBytes(v) },
    },
    yAxis: {
      type: 'category',
      data: [...list].reverse().map((d) => d.name),
      ...ax,
      axisLabel: { color: p.text, fontFamily: 'JetBrains Mono, Consolas, monospace' },
    },
    series: [
      {
        type: 'bar',
        data: [...list].reverse().map((d) => d.up + d.down),
        itemStyle: { color: p.up, borderRadius: [0, 3, 3, 0] },
        barMaxWidth: 14,
      },
    ],
  })
})
</script>

<template>
  <div class="grid-2">
    <div class="panel">
      <h3 class="panel-title">域名流量 TOP 15（本次运行累计）</h3>
      <div v-if="domains.length" ref="domEl" class="chart"></div>
      <div v-else class="empty">暂无数据</div>
    </div>
    <div class="panel">
      <h3 class="panel-title">规则流量 TOP 10（本次运行累计）</h3>
      <div v-if="rules.length" ref="ruleEl" class="chart"></div>
      <div v-else class="empty">暂无数据</div>
    </div>
  </div>
</template>
