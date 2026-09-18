import { reactive } from 'vue'

export interface ConnJSON {
  id: number
  protocol: string
  host: string
  port: string
  rule: string
  action: string
  start: number
  up: number
  down: number
}

export interface NamedAgg {
  name: string
  up: number
  down: number
  conns: number
}

export interface RatePoint {
  t: number
  up: number
  down: number
}

export interface Overview {
  upRate: number
  downRate: number
  totalUp: number
  totalDown: number
  totalConns: number
  activeConns: number
  uptime: number
  rates: RatePoint[]
}

export interface Snapshot {
  overview: Overview
  connections: ConnJSON[]
  closed: ConnJSON[]
  topDomains: NamedAgg[]
  topRules: NamedAgg[]
}

export const wsState = reactive<{ connected: boolean; snap: Snapshot | null }>({
  connected: false,
  snap: null,
})

let retry = 0
let timer: ReturnType<typeof setTimeout> | null = null

function connect() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const ws = new WebSocket(`${proto}://${location.host}/ws`)

  ws.onopen = () => {
    retry = 0
    wsState.connected = true
  }
  ws.onmessage = (ev) => {
    try {
      wsState.snap = JSON.parse(ev.data) as Snapshot
    } catch {
      /* 忽略坏帧 */
    }
  }
  ws.onclose = () => {
    wsState.connected = false
    retry = Math.min(retry + 1, 6)
    timer = setTimeout(connect, 500 * retry)
  }
  ws.onerror = () => ws.close()
}

if (timer) clearTimeout(timer)
connect()
