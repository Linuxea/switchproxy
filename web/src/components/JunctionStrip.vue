<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { wsState } from '../composables/useWs'
import { useConnRates } from '../composables/useConnRates'
import { fmtRate } from '../lib/format'

// 道岔总览：入站流量 → DIRECT / PROXY 两条轨的实时走向图。
// 每轨速率由前端对活跃连接按 action 聚合（与连接页同一差分源），无后端改动。

const rates = useConnRates()

const cfg = ref<{ listen: string; upstreamAddr: string } | null>(null)

// 上级代理是否已配置（决定 PROXY 轨的生死态展示）
const hasUpstream = computed(() => !!cfg.value?.upstreamAddr)
const addrLabel = computed(() => {
  const a = cfg.value?.upstreamAddr ?? ''
  return a.length > 24 ? `${a.slice(0, 23)}…` : a
})

interface Branch {
  up: number
  down: number
  n: number
}

const per = computed<{ DIRECT: Branch; PROXY: Branch }>(() => {
  const d: Branch = { up: 0, down: 0, n: 0 }
  const p: Branch = { up: 0, down: 0, n: 0 }
  for (const c of wsState.snap?.connections ?? []) {
    const r = rates.get(c.id) ?? { up: 0, down: 0 }
    const t = c.action === 'PROXY' ? p : d
    t.up += r.up
    t.down += r.down
    t.n++
  }
  return { DIRECT: d, PROXY: p }
})

// 近期拒绝计数（来自最近关闭列表；REJECT 连接瞬时关闭，只在这里可见）
const rejectCount = computed(
  () => (wsState.snap?.closed ?? []).filter((c) => c.action === 'REJECT').length,
)

// 速率 → 流动速度（px/s）：对数刻度，10 B/s ≈ 12，10 MB/s ≈ 198，封顶 200
function speedOf(bps: number): number {
  if (bps <= 0) return 0
  return Math.min(200, 12 + (Math.log10(bps) - 1) * 31)
}

const dashTrunk = ref<SVGPathElement | null>(null)
const dashDirect = ref<SVGPathElement | null>(null)
const dashProxy = ref<SVGPathElement | null>(null)

const reduced = window.matchMedia('(prefers-reduced-motion: reduce)')
let raf = 0
let last = performance.now()
const off = { trunk: 0, direct: 0, proxy: 0 }
const PERIOD = 14 // 与 stroke-dasharray "3 11" 的周期一致，取模防浮点漂移

function frame(now: number) {
  const dt = Math.min(0.1, (now - last) / 1000)
  last = now
  if (!reduced.matches && wsState.connected) {
    const { DIRECT: d, PROXY: p } = per.value
    const px = hasUpstream.value ? p.down + p.up : 0
    off.trunk = (off.trunk - speedOf(d.down + d.up + px) * dt) % PERIOD
    off.direct = (off.direct - speedOf(d.down + d.up) * dt) % PERIOD
    off.proxy = (off.proxy - speedOf(px) * dt) % PERIOD
    if (dashTrunk.value) dashTrunk.value.style.strokeDashoffset = `${off.trunk}`
    if (dashDirect.value) dashDirect.value.style.strokeDashoffset = `${off.direct}`
    if (dashProxy.value) dashProxy.value.style.strokeDashoffset = `${off.proxy}`
  }
  raf = requestAnimationFrame(frame)
}

onMounted(async () => {
  raf = requestAnimationFrame(frame)
  try {
    const res = await fetch('/api/config')
    if (res.ok) {
      const data = await res.json()
      cfg.value = {
        listen: String(data.listen || ''),
        upstreamAddr: String(data.upstream?.addr || '').trim(),
      }
    }
  } catch {
    /* 读不到配置时按未知处理：分支照常显示速率 */
  }
})

onBeforeUnmount(() => cancelAnimationFrame(raf))
</script>

<template>
  <div class="panel junction" :class="{ offline: !wsState.connected }">
    <!-- 桌面版：SVG 轨道图 -->
    <svg
      class="jxn-svg"
      viewBox="0 0 1000 180"
      role="img"
      aria-label="流量走向：应用入站经道岔分为直连与上级代理两路"
    >
      <!-- 入站节点 -->
      <circle class="node" cx="34" cy="88" r="5" />
      <text class="node-label" x="34" y="64" text-anchor="middle">应用</text>
      <text class="node-addr" x="34" y="124" text-anchor="middle">{{ cfg?.listen || '入站' }}</text>

      <!-- 干线 -->
      <path class="track" d="M 39 88 H 224" />
      <path class="flow flow-trunk" ref="dashTrunk" d="M 39 88 H 224" />
      <path class="frog" d="M 230 81 L 237 88 L 230 95 L 223 88 Z" />

      <!-- DIRECT 轨 -->
      <path class="track" d="M 233 88 C 300 88 306 34 380 34 H 686" />
      <path class="flow flow-direct" ref="dashDirect" d="M 233 88 C 300 88 306 34 380 34 H 686" />
      <path class="arrow a-direct" d="M 686 28 L 698 34 L 686 40 Z" />
      <rect class="lp-direct" x="716" y="30.5" width="7" height="7" rx="1.5" />
      <text class="term-name" x="730" y="39">
        <tspan class="t-strong">DIRECT</tspan><tspan class="t-dim"> · 直连出网</tspan>
      </text>
      <text class="term-data" x="730" y="58">
        <tspan class="t-down">↓ {{ fmtRate(per.DIRECT.down) }}</tspan>
        <tspan class="t-up"> ↑ {{ fmtRate(per.DIRECT.up) }}</tspan>
        <tspan class="t-dim"> · {{ per.DIRECT.n }} 条</tspan>
      </text>

      <!-- PROXY 轨 -->
      <path class="track" :class="{ dead: !hasUpstream }" d="M 233 88 C 300 88 306 128 380 128 H 686" />
      <path v-if="hasUpstream" class="flow flow-proxy" ref="dashProxy" d="M 233 88 C 300 88 306 128 380 128 H 686" />
      <path class="arrow" :class="hasUpstream ? 'a-proxy' : 'a-dead'" d="M 686 122 L 698 128 L 686 134 Z" />
      <rect class="lp" :class="hasUpstream ? 'lp-proxy' : 'lp-dead'" x="716" y="124.5" width="7" height="7" rx="1.5" />
      <text class="term-name" x="730" y="133">
        <tspan class="t-strong">PROXY</tspan>
        <tspan v-if="hasUpstream" class="t-dim"> · {{ addrLabel }}</tspan>
        <tspan v-else class="t-dim"> · 未配置（设置页可添加）</tspan>
      </text>
      <text v-if="hasUpstream" class="term-data" x="730" y="152">
        <tspan class="t-down">↓ {{ fmtRate(per.PROXY.down) }}</tspan>
        <tspan class="t-up"> ↑ {{ fmtRate(per.PROXY.up) }}</tspan>
        <tspan class="t-dim"> · {{ per.PROXY.n }} 条</tspan>
      </text>

      <!-- REJECT 短轨：近期有拒绝才出现 -->
      <g v-if="rejectCount > 0">
        <path class="track" d="M 233 88 C 275 88 276 162 322 162 H 396" />
        <path class="arrow a-reject" d="M 396 156 L 408 162 L 396 168 Z" />
        <rect class="lp-reject" x="426" y="158.5" width="7" height="7" rx="1.5" />
        <text class="term-data t-reject" x="440" y="167">
          <tspan class="t-strong">REJECT</tspan> · 近期 {{ rejectCount }} 次
        </text>
      </g>
    </svg>

    <!-- 窄屏版：SVG 缩放后文字过小，改列表呈现同样的信息 -->
    <div class="jxn-list">
      <div class="jxn-row">
        <span class="jxn-name"><span class="d-green"></span>DIRECT · 直连出网</span>
        <span class="jxn-data">
          <span class="t-down">↓ {{ fmtRate(per.DIRECT.down) }}</span>
          <span class="t-up"> ↑ {{ fmtRate(per.DIRECT.up) }}</span>
          <span class="t-dim"> · {{ per.DIRECT.n }} 条</span>
        </span>
      </div>
      <div class="jxn-row">
        <span class="jxn-name">
          <span :class="hasUpstream ? 'd-amber' : 'd-dead'"></span>
          <template v-if="hasUpstream">PROXY · {{ addrLabel }}</template>
          <template v-else>PROXY · 未配置</template>
        </span>
        <span v-if="hasUpstream" class="jxn-data">
          <span class="t-down">↓ {{ fmtRate(per.PROXY.down) }}</span>
          <span class="t-up"> ↑ {{ fmtRate(per.PROXY.up) }}</span>
          <span class="t-dim"> · {{ per.PROXY.n }} 条</span>
        </span>
      </div>
      <div v-if="rejectCount > 0" class="jxn-row">
        <span class="jxn-name"><span class="d-red"></span>REJECT</span>
        <span class="jxn-data t-reject">近期 {{ rejectCount }} 次</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.junction {
  padding: 10px 16px 4px;
}

.junction.offline .jxn-svg,
.junction.offline .jxn-list {
  opacity: 0.55;
}

.jxn-svg {
  display: block;
  width: 100%;
  height: auto;
}

/* 轨道与节点 */
.track {
  fill: none;
  stroke: var(--track);
  stroke-width: 1.5;
}

.track.dead {
  stroke-dasharray: 2 4;
  opacity: 0.7;
}

.node {
  fill: var(--ink);
}

.frog {
  fill: var(--dim);
}

.arrow {
  fill: var(--dim);
}

.arrow.a-direct {
  fill: var(--green);
}

.arrow.a-proxy {
  fill: var(--amber);
}

.arrow.a-dead {
  fill: var(--line);
}

.arrow.a-reject {
  fill: var(--red);
}

/* 流动短划：速度由真实速率驱动（见 rAF 循环） */
.flow {
  fill: none;
  stroke-width: 2;
  stroke-dasharray: 3 11;
}

.flow-trunk {
  stroke: var(--dim);
}

.flow-direct {
  stroke: var(--green);
}

.flow-proxy {
  stroke: var(--amber);
}

/* 信号灯片 */
.lp {
  fill: var(--dim);
}

.lp-direct {
  fill: var(--green);
}

.lp-proxy {
  fill: var(--amber);
}

.lp-dead {
  fill: var(--track);
}

.lp-reject {
  fill: var(--red);
}

/* 文本 */
.node-label {
  font-family: var(--font-ui);
  font-size: 13px;
  fill: var(--text);
}

.node-addr {
  font-family: var(--font-mono);
  font-size: 10.5px;
  fill: var(--dim);
}

.term-name {
  font-family: var(--font-mono);
  font-size: 13px;
}

.term-data {
  font-family: var(--font-mono);
  font-size: 12px;
  fill: var(--dim);
}

.t-strong {
  font-weight: 600;
  fill: var(--ink);
}

.t-dim {
  fill: var(--dim);
}

.t-down {
  fill: var(--green);
}

.t-up {
  fill: var(--amber);
}

.t-reject {
  fill: var(--red);
}

/* 窄屏列表 */
.jxn-list {
  display: none;
}

.jxn-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  padding: 3px 0;
}

.jxn-name {
  font-family: var(--font-mono);
  font-size: 12.5px;
  color: var(--ink);
  display: inline-flex;
  align-items: center;
  gap: 7px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.jxn-data {
  font-family: var(--font-mono);
  font-size: 12px;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.jxn-list .t-dim {
  color: var(--dim);
  fill: unset;
}

.jxn-list .t-down {
  color: var(--green);
  fill: unset;
}

.jxn-list .t-up {
  color: var(--amber);
  fill: unset;
}

.jxn-list .t-reject {
  color: var(--red);
  fill: unset;
}

.d-green,
.d-amber,
.d-red,
.d-dead {
  width: 7px;
  height: 7px;
  border-radius: 1.5px;
  flex: none;
}

.d-green {
  background: var(--green);
}

.d-amber {
  background: var(--amber);
}

.d-red {
  background: var(--red);
}

.d-dead {
  background: var(--track);
}

@media (max-width: 640px) {
  .jxn-svg {
    display: none;
  }

  .jxn-list {
    display: block;
    padding: 2px 0 6px;
  }
}
</style>
