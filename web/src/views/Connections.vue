<script setup lang="ts">
import { computed } from 'vue'
import { wsState } from '../composables/useWs'
import { useConnRates } from '../composables/useConnRates'
import { themeState } from '../composables/useTheme'
import { fmtBytes, fmtDur, fmtRate, fmtTime } from '../lib/format'

const rates = useConnRates()

// 「新连接」窗口：5 分钟内显示 NEW 徽标，背景不透明度随年龄线性衰减（越新越深）
const NEW_MS = 5 * 60 * 1000

const activeRows = computed(() => {
  const conns = wsState.snap?.connections ?? []
  const now = Date.now()
  const green = themeState.dark ? '52,211,153' : '5,150,105'
  return conns.map((c) => {
    const duration = now - c.start
    const isNew = duration < NEW_MS
    const alpha = 0.9 - (duration / NEW_MS) * 0.75 // 0.9 -> 0.15
    return {
      ...c,
      rate: rates.get(c.id) ?? { up: 0, down: 0 },
      duration,
      isNew,
      newBg: isNew ? `rgba(${green},${alpha.toFixed(3)})` : '',
    }
  })
})

const closedRows = computed(() => wsState.snap?.closed ?? [])

async function closeConn(id: number) {
  try {
    await fetch(`/api/connections/${id}/close`, { method: 'POST' })
  } catch {
    /* 连接可能已自行结束 */
  }
}
</script>

<template>
  <div class="panel">
    <h3 class="panel-title">活跃连接（{{ activeRows.length }}）</h3>
    <div v-if="activeRows.length" class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>协议</th>
            <th>目标</th>
            <th>规则</th>
            <th>动作</th>
            <th>↓ 速率</th>
            <th>↑ 速率</th>
            <th>↓ 流量</th>
            <th>↑ 流量</th>
            <th>时长</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in activeRows" :key="c.id">
            <td class="mono">{{ c.id }}</td>
            <td><span class="badge" :class="c.protocol.toLowerCase()">{{ c.protocol }}</span></td>
            <td class="mono">{{ c.host }}<span class="dim">:{{ c.port }}</span></td>
            <td class="mono">{{ c.rule }}</td>
            <td><span class="badge" :class="c.action.toLowerCase()">{{ c.action }}</span></td>
            <td>{{ fmtRate(c.rate.down) }}</td>
            <td>{{ fmtRate(c.rate.up) }}</td>
            <td>{{ fmtBytes(c.down) }}</td>
            <td>{{ fmtBytes(c.up) }}</td>
            <td>
              <span v-if="c.isNew" class="badge new" :style="{ background: c.newBg }">NEW</span>{{ fmtDur(c.duration) }}
            </td>
            <td><button class="btn-close" @click="closeConn(c.id)">关闭</button></td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-else class="empty">暂无活跃连接</div>
  </div>

  <div class="panel">
    <h3 class="panel-title">最近关闭（{{ closedRows.length }}）</h3>
    <div v-if="closedRows.length" class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>协议</th>
            <th>目标</th>
            <th>规则</th>
            <th>动作</th>
            <th>↓ 流量</th>
            <th>↑ 流量</th>
            <th>时长</th>
            <th>开始于</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in closedRows" :key="c.id">
            <td class="mono">{{ c.id }}</td>
            <td><span class="badge" :class="c.protocol.toLowerCase()">{{ c.protocol }}</span></td>
            <td class="mono">{{ c.host }}<span class="dim">:{{ c.port }}</span></td>
            <td class="mono">{{ c.rule }}</td>
            <td><span class="badge" :class="c.action.toLowerCase()">{{ c.action }}</span></td>
            <td>{{ fmtBytes(c.down) }}</td>
            <td>{{ fmtBytes(c.up) }}</td>
            <td>{{ fmtDur(Date.now() - c.start) }}</td>
            <td>{{ fmtTime(c.start) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-else class="empty">暂无历史连接</div>
  </div>
</template>

<style scoped>
.dim {
  color: var(--text-dim);
}
.table-wrap {
  overflow-x: auto;
}
.badge.new {
  color: var(--green);
  font-size: 10px;
  font-weight: 700;
  padding: 0 6px;
  margin-right: 6px;
  border-radius: 8px;
}
</style>
