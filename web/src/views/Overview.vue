<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import * as echarts from 'echarts'
import { wsState } from '../composables/useWs'
import { useChart, axisCommon } from '../lib/chart'
import { fmtBytes, fmtRate, fmtUptime } from '../lib/format'

const el = ref<HTMLElement | null>(null)
const { apply } = useChart(el)

const ov = computed(() => wsState.snap?.overview ?? null)

watch(ov, (o) => {
  if (!o) return
  const times = o.rates.map((p) => {
    const d = new Date(p.t * 1000)
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
  })
  apply({
    animation: false,
    grid: { left: 8, right: 12, top: 34, bottom: 4, containLabel: true },
    tooltip: {
      trigger: 'axis',
      valueFormatter: (v) => fmtRate(Number(v)),
    },
    legend: { data: ['下行', '上行'], textStyle: { color: '#9aa4b2' }, top: 4, right: 8 },
    xAxis: { type: 'category', boundaryGap: false, data: times, ...axisCommon },
    yAxis: {
      type: 'value',
      ...axisCommon,
      axisLabel: { ...axisCommon.axisLabel, formatter: (v: number) => fmtBytes(v) },
    },
    series: [
      {
        name: '下行',
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: o.rates.map((p) => p.down),
        lineStyle: { width: 1.5, color: '#22d3ee' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(34,211,238,0.28)' },
            { offset: 1, color: 'rgba(34,211,238,0)' },
          ]),
        },
      },
      {
        name: '上行',
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: o.rates.map((p) => p.up),
        lineStyle: { width: 1.5, color: '#f59e0b' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(245,158,11,0.22)' },
            { offset: 1, color: 'rgba(245,158,11,0)' },
          ]),
        },
      },
    ],
  })
})
</script>

<template>
  <div v-if="ov">
    <div class="cards">
      <div class="card">
        <div class="label">下行速率</div>
        <div class="value down">{{ fmtRate(ov.downRate) }}</div>
      </div>
      <div class="card">
        <div class="label">上行速率</div>
        <div class="value up">{{ fmtRate(ov.upRate) }}</div>
      </div>
      <div class="card">
        <div class="label">活跃连接</div>
        <div class="value green">{{ ov.activeConns }}</div>
      </div>
      <div class="card">
        <div class="label">累计连接</div>
        <div class="value">{{ ov.totalConns }}</div>
      </div>
      <div class="card">
        <div class="label">累计下行</div>
        <div class="value down">{{ fmtBytes(ov.totalDown) }}</div>
      </div>
      <div class="card">
        <div class="label">累计上行</div>
        <div class="value up">{{ fmtBytes(ov.totalUp) }}</div>
      </div>
      <div class="card">
        <div class="label">运行时间</div>
        <div class="value">{{ fmtUptime(ov.uptime) }}</div>
      </div>
    </div>
    <div class="panel">
      <h3 class="panel-title">实时速率（近 {{ ov.rates.length }} 秒）</h3>
      <div ref="el" class="chart"></div>
    </div>
  </div>
  <div v-else class="empty">等待数据推送…</div>
</template>
