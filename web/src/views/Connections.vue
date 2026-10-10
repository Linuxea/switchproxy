<script setup lang="ts">
import { computed } from 'vue'
import { wsState } from '../composables/useWs'
import { useConnRates } from '../composables/useConnRates'
import { themeState } from '../composables/useTheme'
import { fmtBytes, fmtDur, fmtRate, fmtTime } from '../lib/format'

const rates = useConnRates()

// 「新连接」窗口：5 分钟内显示 NEW 标记，背景不透明度随年龄线性衰减（越新越深）
const NEW_MS = 5 * 60 * 1000

const activeRows = computed(() => {
  const conns = wsState.snap?.connections ?? []
  const now = Date.now()
  const green = themeState.dark ? '53,201,138' : '12,127,76'
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
    <h3 class="panel-title">活跃连接 · {{ activeRows.length }}</h3>
    <div v-if="activeRows.length" class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>协议</th>
            <th>目标</th>
            <th>规则</th>
            <th>动作</th>
            <th class="num">↓ 速率</th>
            <th class="num">↑ 速率</th>
            <th class="num">↓ 流量</th>
            <th class="num">↑ 流量</th>
            <th class="num">时长</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in activeRows" :key="c.id">
            <td class="mono dim">{{ c.id }}</td>
            <td class="mono dim">{{ c.protocol }}</td>
            <td class="mono">{{ c.host }}<span class="dim">:{{ c.port }}</span></td>
            <td class="mono dim">{{ c.rule }}</td>
            <td>
              <span class="lamp" :class="c.action.toLowerCase()">
                <span class="dot"></span>{{ c.action }}
              </span>
            </td>
            <td class="num">{{ fmtRate(c.rate.down) }}</td>
            <td class="num">{{ fmtRate(c.rate.up) }}</td>
            <td class="num">{{ fmtBytes(c.down) }}</td>
            <td class="num">{{ fmtBytes(c.up) }}</td>
            <td class="num">
              <span v-if="c.isNew" class="new-tag" :style="{ color: c.newBg }">NEW</span>{{ fmtDur(c.duration) }}
            </td>
            <td><button class="btn-close" @click="closeConn(c.id)">关闭</button></td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-else class="empty">暂无活跃连接 — 将系统代理指向入站地址后，连接会出现在这里</div>
  </div>

  <div class="panel">
    <h3 class="panel-title">最近关闭 · {{ closedRows.length }}</h3>
    <div v-if="closedRows.length" class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>协议</th>
            <th>目标</th>
            <th>规则</th>
            <th>动作</th>
            <th class="num">↓ 流量</th>
            <th class="num">↑ 流量</th>
            <th class="num">时长</th>
            <th class="num">开始于</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in closedRows" :key="c.id">
            <td class="mono dim">{{ c.id }}</td>
            <td class="mono dim">{{ c.protocol }}</td>
            <td class="mono">{{ c.host }}<span class="dim">:{{ c.port }}</span></td>
            <td class="mono dim">{{ c.rule }}</td>
            <td>
              <span class="lamp" :class="c.action.toLowerCase()">
                <span class="dot"></span>{{ c.action }}
              </span>
            </td>
            <td class="num">{{ fmtBytes(c.down) }}</td>
            <td class="num">{{ fmtBytes(c.up) }}</td>
            <td class="num">{{ fmtDur(Date.now() - c.start) }}</td>
            <td class="num">{{ fmtTime(c.start) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-else class="empty">暂无关闭记录</div>
  </div>
</template>

<style scoped>
.dim {
  color: var(--dim);
}

.table-wrap {
  overflow-x: auto;
}

.new-tag {
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.04em;
  margin-right: 6px;
}
</style>
