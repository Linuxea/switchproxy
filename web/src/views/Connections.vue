<script setup lang="ts">
import { computed } from 'vue'
import { wsState } from '../composables/useWs'
import { useConnRates } from '../composables/useConnRates'
import { fmtBytes, fmtDur, fmtRate, fmtTime } from '../lib/format'

const rates = useConnRates()

const activeRows = computed(() => {
  const conns = wsState.snap?.connections ?? []
  const now = Date.now()
  return conns.map((c) => ({
    ...c,
    rate: rates.get(c.id) ?? { up: 0, down: 0 },
    duration: now - c.start,
  }))
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
            <td>{{ fmtDur(c.duration) }}</td>
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
</style>
