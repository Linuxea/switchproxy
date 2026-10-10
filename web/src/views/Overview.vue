<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { wsState } from '../composables/useWs'
import { useChart, axisCommon, chartPalette } from '../lib/chart'
import { fmtBytes, fmtRate, fmtUptime } from '../lib/format'
import JunctionStrip from '../components/JunctionStrip.vue'

const el = ref<HTMLElement | null>(null)
const { apply } = useChart(el)

const ov = computed(() => wsState.snap?.overview ?? null)

watch(ov, (o) => {
  if (!o) return
  const ax = axisCommon()
  const cp = chartPalette()
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
    legend: { data: ['下行', '上行'], textStyle: { color: cp.legend, fontFamily: 'IBM Plex Sans, PingFang SC, sans-serif' }, top: 4, right: 8 },
    xAxis: { type: 'category', boundaryGap: false, data: times, ...ax },
    yAxis: {
      type: 'value',
      ...ax,
      axisLabel: { ...ax.axisLabel, formatter: (v: number) => fmtBytes(v) },
    },
    series: [
      {
        name: '下行',
        type: 'line',
        smooth: false, // 走纸记录仪：直线段描迹，不平滑
        symbol: 'none',
        data: o.rates.map((p) => p.down),
        lineStyle: { width: 1.5, color: cp.down },
        areaStyle: { color: cp.downArea },
      },
      {
        name: '上行',
        type: 'line',
        smooth: false,
        symbol: 'none',
        data: o.rates.map((p) => p.up),
        lineStyle: { width: 1.5, color: cp.up },
        areaStyle: { color: cp.upArea },
      },
    ],
  })
})
</script>

<template>
  <div v-if="ov">
    <JunctionStrip />

    <div class="readouts">
      <div class="readout major">
        <div class="k"><span class="sig down">↓</span>下行速率</div>
        <div class="v down">{{ fmtRate(ov.downRate) }}</div>
      </div>
      <div class="readout major">
        <div class="k"><span class="sig up">↑</span>上行速率</div>
        <div class="v up">{{ fmtRate(ov.upRate) }}</div>
      </div>
      <div class="readout">
        <div class="k">活跃连接</div>
        <div class="v">{{ ov.activeConns }}</div>
      </div>
      <div class="readout">
        <div class="k">累计连接</div>
        <div class="v">{{ ov.totalConns.toLocaleString() }}</div>
      </div>
      <div class="readout">
        <div class="k">累计下行</div>
        <div class="v">{{ fmtBytes(ov.totalDown) }}</div>
      </div>
      <div class="readout">
        <div class="k">累计上行</div>
        <div class="v">{{ fmtBytes(ov.totalUp) }}</div>
      </div>
      <div class="readout">
        <div class="k">运行时间</div>
        <div class="v">{{ fmtUptime(ov.uptime) }}</div>
      </div>
    </div>

    <div class="panel">
      <h3 class="panel-title">实时速率 · 近 {{ ov.rates.length }} 秒</h3>
      <div ref="el" class="chart"></div>
    </div>
  </div>
  <div v-else class="empty">等待数据推送…</div>
</template>
