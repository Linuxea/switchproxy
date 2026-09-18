<script setup lang="ts">
import { onMounted, ref } from 'vue'
import * as echarts from 'echarts'
import { useChart, axisCommon } from '../lib/chart'
import { fmtBytes } from '../lib/format'

interface HistoryPoint {
  t: number
  up: number
  down: number
  conns: number
}
interface HistoryDomain {
  domain: string
  up: number
  down: number
  conns: number
}
interface HistoryResult {
  range: string
  points: HistoryPoint[]
  domains: HistoryDomain[]
}

const ranges = [
  { key: '1h', label: '1 小时' },
  { key: '24h', label: '24 小时' },
  { key: '7d', label: '7 天' },
  { key: '30d', label: '30 天' },
]
const current = ref('24h')
const result = ref<HistoryResult | null>(null)
const loading = ref(false)

const el = ref<HTMLElement | null>(null)
const { apply } = useChart(el)

function timeLabel(t: number, rng: string) {
  const d = new Date(t * 1000)
  const p = (n: number) => String(n).padStart(2, '0')
  if (rng === '1h') return `${p(d.getHours())}:${p(d.getMinutes())}`
  if (rng === '24h') return `${p(d.getHours())}:${p(d.getMinutes())}`
  return `${d.getMonth() + 1}/${d.getDate()} ${p(d.getHours())}时`
}

function render() {
  if (!result.value) return
  const pts = result.value.points
  apply({
    animation: false,
    grid: { left: 8, right: 12, top: 34, bottom: 4, containLabel: true },
    tooltip: {
      trigger: 'axis',
      valueFormatter: (v) => fmtBytes(Number(v)),
    },
    legend: { data: ['下行', '上行'], textStyle: { color: '#9aa4b2' }, top: 4, right: 8 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: pts.map((p) => timeLabel(p.t, result.value!.range)),
      ...axisCommon,
    },
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
        data: pts.map((p) => p.down),
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
        data: pts.map((p) => p.up),
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
}

async function load() {
  loading.value = true
  try {
    const res = await fetch(`/api/history?range=${current.value}`)
    result.value = (await res.json()) as HistoryResult
    render()
  } finally {
    loading.value = false
  }
}

function pick(rng: string) {
  current.value = rng
  load()
}

onMounted(load)
</script>

<template>
  <div class="panel">
    <div class="toolbar">
      <button v-for="r in ranges" :key="r.key" :class="{ active: current === r.key }" @click="pick(r.key)">
        {{ r.label }}
      </button>
      <span style="flex: 1"></span>
      <button @click="load">刷新</button>
    </div>
    <div ref="el" class="chart"></div>
    <div v-if="result && result.points.length === 0 && !loading" class="empty">
      该时间段暂无历史数据（每分钟落盘一次）
    </div>
  </div>

  <div class="panel" v-if="result && result.domains.length">
    <h3 class="panel-title">域名流量排行（区间汇总）</h3>
    <table>
      <thead>
        <tr>
          <th>#</th>
          <th>域名</th>
          <th>↓ 流量</th>
          <th>↑ 流量</th>
          <th>连接数</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(d, i) in result.domains" :key="d.domain">
          <td>{{ i + 1 }}</td>
          <td class="mono">{{ d.domain }}</td>
          <td>{{ fmtBytes(d.down) }}</td>
          <td>{{ fmtBytes(d.up) }}</td>
          <td>{{ d.conns }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
