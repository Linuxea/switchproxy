import * as echarts from 'echarts'
import { onBeforeUnmount, onMounted, shallowRef, type Ref } from 'vue'
import { themeState } from '../composables/useTheme'

export function useChart(el: Ref<HTMLElement | null>) {
  const chart = shallowRef<echarts.ECharts | null>(null)

  const apply = (opt: echarts.EChartsOption) => {
    chart.value?.setOption(opt, { notMerge: true })
  }

  const onResize = () => chart.value?.resize()

  onMounted(() => {
    if (el.value) {
      chart.value = echarts.init(el.value)
      window.addEventListener('resize', onResize)
    }
  })

  onBeforeUnmount(() => {
    window.removeEventListener('resize', onResize)
    chart.value?.dispose()
    chart.value = null
  })

  return { apply }
}

// 主题感知的图表配色：信号语义（绿=下行/放行、黄=上行/绕行），与 CSS token 同源。
// 视图每次渲染时取当前值，切主题后由 App 层 :key 重挂载重绘。
export function chartPalette() {
  return themeState.dark
    ? {
        text: '#b9bec4',
        axisLine: '#33363b',
        axisLabel: '#7f858c',
        splitLine: '#2a2d31',
        legend: '#7f858c',
        down: '#35c98a',
        downArea: 'rgba(53, 201, 138, 0.13)',
        up: '#eda23c',
        upArea: 'rgba(237, 162, 60, 0.11)',
        mono: 'IBM Plex Mono, PingFang SC, Consolas, monospace',
      }
    : {
        text: '#3a3f45',
        axisLine: '#d5d9dd',
        axisLabel: '#71767d',
        splitLine: '#e4e7ea',
        legend: '#71767d',
        down: '#0c7f4c',
        downArea: 'rgba(12, 127, 76, 0.11)',
        up: '#b35c07',
        upArea: 'rgba(179, 92, 7, 0.10)',
        mono: 'IBM Plex Mono, PingFang SC, Consolas, monospace',
      }
}

// 通用坐标轴配置：虚线细网格，走纸记录仪质感
export function axisCommon() {
  const p = chartPalette()
  return {
    axisLine: { lineStyle: { color: p.axisLine } },
    axisLabel: { color: p.axisLabel, fontFamily: p.mono },
    splitLine: { lineStyle: { color: p.splitLine, type: 'dashed' as const } },
  }
}
