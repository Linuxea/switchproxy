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

// 主题感知的图表配色：视图每次渲染时取当前值，切主题后由 App 层 :key 重挂载重绘。
export function chartPalette() {
  return themeState.dark
    ? {
        text: '#c7d0dd',
        axisLine: '#3a4150',
        axisLabel: '#7d8794',
        splitLine: '#232a36',
        legend: '#9aa4b2',
        down: '#22d3ee',
        downArea: ['rgba(34,211,238,0.28)', 'rgba(34,211,238,0)'],
        up: '#f59e0b',
        upArea: ['rgba(245,158,11,0.22)', 'rgba(245,158,11,0)'],
      }
    : {
        text: '#24292f',
        axisLine: '#c7ced9',
        axisLabel: '#6b7280',
        splitLine: '#e5e9f0',
        legend: '#6b7280',
        down: '#0891b2',
        downArea: ['rgba(8,145,178,0.2)', 'rgba(8,145,178,0)'],
        up: '#d97706',
        upArea: ['rgba(217,119,6,0.18)', 'rgba(217,119,6,0)'],
      }
}

// 通用坐标轴配置（原 axisCommon，改为函数以感知主题）
export function axisCommon() {
  const p = chartPalette()
  return {
    axisLine: { lineStyle: { color: p.axisLine } },
    axisLabel: { color: p.axisLabel },
    splitLine: { lineStyle: { color: p.splitLine } },
  }
}
