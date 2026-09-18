import * as echarts from 'echarts'
import { onBeforeUnmount, onMounted, shallowRef, type Ref } from 'vue'

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

// 通用暗色主题坐标轴配置
export const axisCommon = {
  axisLine: { lineStyle: { color: '#3a4150' } },
  axisLabel: { color: '#7d8794' },
  splitLine: { lineStyle: { color: '#232a36' } },
}
