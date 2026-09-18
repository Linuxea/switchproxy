import { reactive } from 'vue'
import type { Snapshot } from './useWs'
import { wsState } from './useWs'

// 模块级状态：跨视图切换保留各连接的上一秒快照，用于计算单连接实时速率
const prev = new Map<number, { up: number; down: number; t: number }>()
export const connRates = reactive(new Map<number, { up: number; down: number }>())

let watching = false

export function useConnRates() {
  if (!watching) {
    watching = true
    let lastSnap: Snapshot | null = null
    let lastT = 0
    const compute = () => {
      const snap = wsState.snap
      if (!snap || snap === lastSnap) return
      lastSnap = snap
      const now = Date.now()
      const seen = new Set<number>()
      for (const c of snap.connections) {
        seen.add(c.id)
        const p = prev.get(c.id)
        if (p && now > p.t) {
          const dt = (now - p.t) / 1000
          connRates.set(c.id, {
            up: Math.max(0, (c.up - p.up) / dt),
            down: Math.max(0, (c.down - p.down) / dt),
          })
        }
        prev.set(c.id, { up: c.up, down: c.down, t: now })
      }
      for (const id of prev.keys()) {
        if (!seen.has(id)) {
          prev.delete(id)
          connRates.delete(id)
        }
      }
      lastT = now
    }
    void lastT
    setInterval(compute, 500)
  }
  return connRates
}
