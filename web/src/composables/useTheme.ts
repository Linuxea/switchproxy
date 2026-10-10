import { reactive } from 'vue'

// 主题状态：默认跟随系统 prefers-color-scheme，手动切换后覆盖（localStorage 记忆）。
// 单例 + 模块副作用初始化，与 useWs 同款模式：组件只读状态，不拥有逻辑。
// 手动选择通过 html.light / html.dark 类落盘到 DOM，CSS 侧类优先级高于媒体查询。

type Manual = 'light' | 'dark' | null

export const themeState = reactive({ dark: true, manual: null as Manual })

function apply() {
  const root = document.documentElement
  root.classList.remove('light', 'dark')
  if (themeState.manual) root.classList.add(themeState.manual)
  const sysDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  themeState.dark = themeState.manual ? themeState.manual === 'dark' : sysDark
}

const saved = localStorage.getItem('sp-theme')
themeState.manual = saved === 'light' || saved === 'dark' ? saved : null
apply()

window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
  if (!themeState.manual) apply() // 未手动选择时实时跟随系统
})

export function toggleTheme() {
  themeState.manual = themeState.dark ? 'light' : 'dark'
  localStorage.setItem('sp-theme', themeState.manual)
  apply()
}
